package model

import (
	"time"
	"unicode/utf8"
)

// MaxImageMembers is the original's image_member_quota default.
const MaxImageMembers = 128

// Member is one row of the original's image_members table: a project an
// image with visibility "shared" is shared with, and that project's answer.
type Member struct {
	ImageID, MemberID, Status string
	CreatedAt, UpdatedAt      time.Time
}

// MemberStatus reports whether status is one the original accepts.
func MemberStatus(status string) bool { return member(status, "pending accepted rejected") }

// ValidMemberID checks a project ID as the original's member schema does.
func ValidMemberID(id string) error {
	if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > 255 {
		return Fail(Invalid, "Invalid member ID.")
	}
	return nil
}
