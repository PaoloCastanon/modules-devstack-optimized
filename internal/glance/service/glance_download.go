package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/google/uuid"
)

// glance-download is the original's import from another region's Glance
// (glance/async_/flows/_internal_plugins/glance_download.py and
// _ImportMetadata in api_image_import.py): the user's token finds the
// region's image endpoint in the user's service catalog, the remote image's
// metadata sets the formats and the [glance_download_properties] properties,
// and its data is staged and then imported as web-download's.

// SetKeystone enables glance-download: the user's service catalog is read from
// Keystone (GET {url}/auth/catalog with the user's token).
func (m *Importer) SetKeystone(keystoneURL string, client *http.Client) {
	m.keystone, m.keystoneClient = strings.TrimRight(keystoneURL, "/"), client
}

// checkGlanceDownload is the original's request validation (images.py):
// glance_region and glance_image_id are required, the remote ID must be a
// UUID and the interface defaults to public, which the task input records.
func checkGlanceDownload(req *ImportRequest) error {
	var method map[string]json.RawMessage
	if len(req.Raw) > 0 {
		var body struct {
			Method map[string]json.RawMessage `json:"method"`
		}
		if json.Unmarshal(req.Raw, &body) == nil {
			method = body.Method
		}
	}
	has := func(key string) bool {
		if method != nil {
			_, ok := method[key]
			return ok
		}
		return map[string]string{"glance_region": req.Method.GlanceRegion, "glance_image_id": req.Method.GlanceImageID,
			"glance_service_interface": req.Method.GlanceServiceInterface}[key] != ""
	}
	if !has("glance_region") {
		return model.Fail(model.Invalid, "'glance_region' needs to be set for glance-download import method")
	}
	if !has("glance_image_id") {
		return model.Fail(model.Invalid, "'glance_image_id' needs to be set for glance-download import method")
	}
	if _, err := uuid.Parse(req.Method.GlanceImageID); err != nil {
		return model.Fail(model.Invalid, "Remote image id does not look like a UUID: "+req.Method.GlanceImageID)
	}
	if !has("glance_service_interface") {
		req.Method.GlanceServiceInterface = "public"
		if len(req.Raw) > 0 {
			var body map[string]any
			if json.Unmarshal(req.Raw, &body) == nil {
				if m, ok := body["method"].(map[string]any); ok {
					m["glance_service_interface"] = "public"
					if raw, err := json.Marshal(body); err == nil {
						req.Raw = raw
					}
				}
			}
		}
	}
	return nil
}

// glanceEndpoint is the original's get_glance_endpoint: the image service's
// endpoint for the region (case-insensitive) and interface.
func (m *Importer) glanceEndpoint(ctx context.Context, token, region, iface string) (string, error) {
	missing := fmt.Errorf("%s glance endpoint not found for region %s", iface, region)
	if m.keystone == "" || m.keystoneClient == nil {
		return "", missing
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.keystone+"/auth/catalog", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Auth-Token", token)
	req.Header.Set("Accept", "application/json")
	res, err := m.keystoneClient.Do(req)
	if err != nil {
		return "", errors.New("the service catalog could not be read from Keystone")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the service catalog could not be read from Keystone (HTTP %d)", res.StatusCode)
	}
	var body struct {
		Catalog []struct {
			Type      string `json:"type"`
			Endpoints []struct {
				Interface, Region, URL string
				RegionID               string `json:"region_id"`
			} `json:"endpoints"`
		} `json:"catalog"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&body); err != nil {
		return "", errors.New("invalid service catalog from Keystone")
	}
	for _, s := range body.Catalog {
		if s.Type != "image" {
			continue
		}
		for _, e := range s.Endpoints {
			r := e.Region
			if r == "" {
				r = e.RegionID
			}
			if strings.EqualFold(r, region) && strings.EqualFold(e.Interface, iface) && e.URL != "" {
				return strings.TrimRight(e.URL, "/"), nil
			}
		}
	}
	return "", missing
}

// remoteGet opens a remote Glance URL with the user's token, through the same
// [import_filtering_opts] filter and redirect checks as web-download.
func (m *Importer) remoteGet(ctx context.Context, raw, token string) (*http.Response, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if err = m.filter(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-Token", token)
	res, err := m.client.Do(req)
	if err != nil {
		return nil, errors.New("glance-download request failed")
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("glance-download returned HTTP %d", res.StatusCode)
	}
	return res, nil
}

// glanceDownload runs _ImportMetadata and _DownloadGlanceImage. On failure
// the formats and properties it set are restored (the original's revert);
// the caller returns the image to queued.
func (m *Importer) glanceDownload(ctx context.Context, id, taskID string, job importJob) (err error) {
	endpoint, err := m.glanceEndpoint(ctx, job.token, job.region, job.iface)
	if err != nil {
		return err
	}
	res, err := m.remoteGet(ctx, endpoint+"/v2/images/"+job.remoteID, job.token)
	if err != nil {
		return err
	}
	var data map[string]any
	err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&data)
	res.Body.Close()
	if err != nil {
		return errors.New("invalid remote image metadata")
	}
	if s, _ := data["status"].(string); s != "active" {
		return fmt.Errorf("Source image status should be active instead of %v", data["status"])
	}
	copied := map[string]string{}
	for key, value := range data {
		for _, prefix := range m.cfg.DownloadProperties {
			if strings.HasPrefix(key, prefix) && !strings.HasPrefix(key, "os_glance") {
				if v, ok := value.(string); ok {
					copied[key] = v
				}
			}
		}
	}
	disk, _ := data["disk_format"].(string)
	container, _ := data["container_format"].(string)
	var before model.Image
	if err = m.setImage(ctx, id, func(i *model.Image) {
		before = model.Image{DiskFormat: i.DiskFormat, ContainerFormat: i.ContainerFormat, Properties: cloneProps(i.Properties)}
		i.DiskFormat, i.ContainerFormat = &disk, &container
		if len(copied) > 0 {
			i.Properties = cloneProps(i.Properties)
			for k, v := range copied {
				i.Properties[k] = v
			}
		}
	}); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = m.setImage(context.WithoutCancel(ctx), id, func(i *model.Image) {
				i.DiskFormat, i.ContainerFormat = before.DiskFormat, before.ContainerFormat
				i.Properties = cloneProps(i.Properties)
				for k := range copied {
					if _, had := before.Properties[k]; !had {
						delete(i.Properties, k)
					}
				}
				for k, v := range before.Properties {
					if _, set := copied[k]; set {
						i.Properties[k] = v
					}
				}
			})
		}
	}()
	size, ok := data["size"].(float64)
	if !ok || size < 0 || size != float64(int64(size)) {
		return fmt.Errorf("Size attribute of remote image %s could not be determined.", job.remoteID)
	}
	if res, err = m.remoteGet(ctx, endpoint+"/v2/images/"+job.remoteID+"/file", job.token); err != nil {
		return err
	}
	defer res.Body.Close()
	n, err := m.writeStaged(ctx, id, res.Body, nil)
	if err != nil {
		return err
	}
	if n != int64(size) {
		return fmt.Errorf("Task %s failed because downloaded data size %d is different from expected %d", taskID, n, int64(size))
	}
	return nil
}
