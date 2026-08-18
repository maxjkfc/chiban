package config_test

import (
	"slices"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/chat"
	"github.com/maxjkfc/chiban/apps/api/internal/config"
	"github.com/maxjkfc/chiban/apps/api/internal/meal"
	"github.com/maxjkfc/chiban/apps/api/internal/profile"
	"github.com/maxjkfc/chiban/apps/api/internal/sticker"
)

// Every domain names its own bucket, and the process creates the buckets on
// this list at startup. Nothing else connects the two, so a domain added with
// a name that is not on the list compiles, passes every test that uses the
// in-memory storage, and then fails on the first real upload — which is
// exactly how the sticker bucket was found, in a browser.
func TestEveryDomainBucketIsProvisioned(t *testing.T) {
	for _, bucket := range []string{chat.Bucket, meal.Bucket, profile.Bucket, sticker.Bucket} {
		if !slices.Contains(config.Buckets, bucket) {
			t.Errorf("bucket %q is used by a domain but never created; add it to config.Buckets", bucket)
		}
	}
}
