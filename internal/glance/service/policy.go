package service

import (
	"context"
	"fmt"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/auth"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/model"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/policy"
)

// Policy is the oslo.policy file of glance-api.conf ([oslo_policy]
// policy_file), set once at startup. Without one (the default, and
// DevStack's) the original's default rules apply as written in this
// package's checks; with one, each action is decided by its rules, with
// the target the original builds (owner as project_id, visibility,
// member_id, the image's properties).
var Policy *policy.Enforcer

func overridden() bool { return Policy.Overridden() }

func credentials(who auth.Identity) policy.Credentials {
	return policy.Credentials{Roles: who.Roles, ProjectID: who.ProjectID, UserID: who.UserID, IsAdmin: who.HasRole("admin")}
}

func denied(action string) error {
	return model.Fail(model.Forbidden, "You are not authorized to complete "+action+" action.")
}

// Permit evaluates action against target when a policy file is in use;
// otherwise it returns def, the default rule's outcome.
func Permit(who auth.Identity, action string, target map[string]any, def error) error {
	if !overridden() {
		return def
	}
	if target == nil {
		target = map[string]any{}
	}
	if _, ok := target["project_id"]; !ok {
		target["project_id"] = who.ProjectID
	}
	if Policy.Allowed(action, target, credentials(who)) {
		return nil
	}
	return denied(action)
}

// imageTarget is the original's ImageTarget.
func (s *Service) imageTarget(ctx context.Context, who auth.Identity, image model.Image) map[string]any {
	t := map[string]any{}
	for k, v := range image.Properties {
		t[k] = v
	}
	t["id"], t["project_id"], t["owner"], t["visibility"], t["status"] = image.ID, image.Owner, image.Owner, image.Visibility, image.Status
	t["protected"] = fmt.Sprint(image.Protected)
	t["member_id"] = ""
	if member, err := s.member(ctx, who, image); err == nil && member {
		t["member_id"] = who.ProjectID
	}
	return t
}

// permitImage is Permit for an image the caller can see.
func (s *Service) permitImage(ctx context.Context, who auth.Identity, action string, image model.Image, def error) error {
	if !overridden() {
		return def
	}
	return Permit(who, action, s.imageTarget(ctx, who, image), def)
}

// visibilityTarget: the target of a create or visibility change.
func visibilityTarget(who auth.Identity, image model.Image) map[string]any {
	return map[string]any{"project_id": image.Owner, "owner": image.Owner, "visibility": image.Visibility, "member_id": ""}
}
