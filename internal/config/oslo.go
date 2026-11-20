package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

// Reading the original's glance-api.conf (GLANCE_GO_CONFIG_FILE). With it,
// Glance Go takes from the same file, as oslo.config reads it, what the
// original uses: [database] connection (same MySQL account), [glance_store]
// filesystem_store_datadir, [keystone_authtoken] (same Keystone service user
// and token_cache_time), [oslo_messaging_notifications] with the [DEFAULT]
// transport_url fallback, default_publisher_id, disabled_notifications and
// public_endpoint. Glance Go's own transfer bookkeeping goes to the
// glance_go_shared database on that same server.

// SharedDatabase is where Glance Go keeps its bookkeeping next to the catalog.
const SharedDatabase = "glance_go_shared"

type osloFile map[string]map[string]string

// parseOslo reads an oslo.config INI file: repeated sections merge, the last
// value of a key wins, keys are case-insensitive, # and ; start comments and
// continuation lines are not used by DevStack's generated files.
func parseOslo(text string) (osloFile, error) {
	out := osloFile{"DEFAULT": {}}
	section := ""
	for n, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			if !strings.HasSuffix(line, "]") {
				return nil, errors.New("glance-api.conf: malformed section on line " + strconv.Itoa(n+1))
			}
			section = strings.TrimSpace(line[1 : len(line)-1])
			if out[section] == nil {
				out[section] = map[string]string{}
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || section == "" {
			return nil, errors.New("glance-api.conf: malformed option on line " + strconv.Itoa(n+1))
		}
		out[section][strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return out, nil
}

// get falls back to [DEFAULT], as oslo.config does for unset group options.
func (f osloFile) get(section, key string) string {
	if v, ok := f[section][key]; ok {
		return v
	}
	return f["DEFAULT"][key]
}

func (f osloFile) list(section, key string) []string {
	var out []string
	for _, item := range strings.Split(f.get(section, key), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// readOriginalConfig reads the file only if it is a regular file owned by
// this user or root and not writable by group or others.
func readOriginalConfig(path string) (osloFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open the original glance-api.conf")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
		return nil, errors.New("the original glance-api.conf must be a regular file not writable by group or others")
	}
	if st, ok := info.Sys().(*syscall.Stat_t); !ok || (st.Uid != uint32(os.Geteuid()) && st.Uid != 0) {
		return nil, errors.New("the original glance-api.conf must belong to this user or root")
	}
	data := make([]byte, 1<<20)
	n, err := f.Read(data)
	if err != nil || n == len(data) {
		return nil, errors.New("cannot read the original glance-api.conf")
	}
	return parseOslo(string(data[:n]))
}

// databaseDSN converts the original's SQLAlchemy URL (mysql+pymysql://user:
// password@host[:port]/glance?...) into a Go DSN for the same account and
// server, on database glance_go_shared. It returns the catalog name too.
func databaseDSN(connection string) (dsn, catalog string, err error) {
	u, err := url.Parse(connection)
	if err != nil || !strings.HasPrefix(u.Scheme, "mysql") || u.User == nil || u.Hostname() == "" {
		return "", "", errors.New("the original [database] connection must be a MySQL URL")
	}
	catalog = strings.TrimPrefix(u.Path, "/")
	password, _ := u.User.Password()
	port := u.Port()
	if port == "" {
		port = "3306"
	}
	c := driver.NewConfig()
	c.User, c.Passwd, c.Net, c.Addr, c.DBName = u.User.Username(), password, "tcp", net.JoinHostPort(u.Hostname(), port), SharedDatabase
	c.ParseTime = true
	return c.FormatDSN(), catalog, nil
}

// applyOriginalConfig fills c from the original's file. Settings that the
// file now provides must not also come from GLANCE_GO_* variables.
func applyOriginalConfig(c *Config, f osloFile, env func(string) string, mode string) error {
	for _, key := range []string{"DATABASE_DSN_FILE", "KEYSTONE_AUTH_URL", "KEYSTONE_CREDENTIAL_FILE", "OFFICIAL_DATABASE", "OFFICIAL_STORE_DIR",
		"NOTIFICATION_DRIVER", "TRANSPORT_URL_FILE", "NOTIFICATION_TOPICS", "DEFAULT_PUBLISHER_ID", "DISABLED_NOTIFICATIONS", "TOKEN_CACHE_TIME"} {
		if env("GLANCE_GO_"+key) != "" {
			return errors.New("GLANCE_GO_" + key + " conflicts with GLANCE_GO_CONFIG_FILE, which provides it")
		}
	}
	if v := env("GLANCE_GO_CATALOG"); v != "" && v != "official" {
		return errors.New("GLANCE_GO_CONFIG_FILE implies GLANCE_GO_CATALOG=official")
	}
	dsn, catalog, err := databaseDSN(f.get("database", "connection"))
	if err != nil {
		return err
	}
	if !officialCatalogName.MatchString(catalog) {
		return errors.New("the original [database] connection must name the glance database")
	}
	c.DatabaseDSN, c.OfficialCatalog, c.OriginalAccount = dsn, catalog, true
	dir := strings.TrimRight(f.get("glance_store", "filesystem_store_datadir"), "/")
	if dir == "" || dir[0] != '/' {
		return errors.New("the original [glance_store] filesystem_store_datadir must be absolute")
	}
	if c.Binary.Backend != "" && c.Binary.Backend != "filesystem" {
		return errors.New("the official catalog is supported only with the filesystem store")
	}
	c.Binary.OfficialStoreDir = dir
	if v := f.get("keystone_authtoken", "token_cache_time"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < -1 || n > 86400 {
			return errors.New("invalid [keystone_authtoken] token_cache_time")
		}
		c.TokenCacheTime = time.Duration(max(n, 0)) * time.Second
	}
	if env("GLANCE_GO_PUBLIC_BASE_URL") == "" {
		if public := strings.TrimRight(f.get("DEFAULT", "public_endpoint"), "/"); public != "" {
			if err := ValidateURL(public); err != nil {
				return errors.New("invalid [DEFAULT] public_endpoint")
			}
			c.PublicURL = public
		}
	}
	if mode != "serve" {
		return nil
	}
	n := Notifications{Driver: "noop", Topics: []string{"notifications"}, PublisherID: "image.localhost", Disabled: map[string]bool{}}
	for _, d := range f.list("oslo_messaging_notifications", "driver") {
		if d == "messagingv2" {
			n.Driver = d
		}
	}
	if n.Driver == "messagingv2" {
		if n.TransportURL = f.get("oslo_messaging_notifications", "transport_url"); n.TransportURL == "" {
			return errors.New("messagingv2 notifications without a transport_url")
		}
	}
	if topics := f.list("oslo_messaging_notifications", "topics"); len(topics) > 0 {
		n.Topics = topics
	}
	if id := f["DEFAULT"]["default_publisher_id"]; id != "" {
		n.PublisherID = id
	}
	for _, d := range f.list("DEFAULT", "disabled_notifications") {
		n.Disabled[d] = true
	}
	for _, name := range append(append([]string{n.PublisherID}, n.Topics...), f.list("DEFAULT", "disabled_notifications")...) {
		if !notificationName.MatchString(name) {
			return errors.New("invalid notification topic, publisher or disabled name in glance-api.conf")
		}
	}
	c.Notifications = n
	k := func(key string) string { return f["keystone_authtoken"][key] }
	if t := k("auth_type"); t != "" && t != "password" {
		return errors.New("only [keystone_authtoken] auth_type = password is supported")
	}
	c.KeystoneURL = strings.TrimRight(k("auth_url"), "/")
	if !strings.HasSuffix(c.KeystoneURL, "/v3") {
		c.KeystoneURL += "/v3"
	}
	if err := ValidateURL(c.KeystoneURL); err != nil {
		return errors.New("invalid [keystone_authtoken] auth_url")
	}
	c.Credential = ServiceCredential{Username: k("username"), Password: k("password"), ProjectName: k("project_name"),
		UserDomainName: firstOf(k("user_domain_name"), "Default"), ProjectDomainName: firstOf(k("project_domain_name"), "Default")}
	if c.Credential.Username == "" || c.Credential.Password == "" || c.Credential.ProjectName == "" {
		return errors.New("[keystone_authtoken] needs username, password and project_name")
	}
	return nil
}

func firstOf(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}
