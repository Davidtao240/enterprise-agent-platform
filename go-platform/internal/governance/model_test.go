package governance

import "testing"

func TestIsSupportedResourceType(t *testing.T) {
	for _, resourceType := range []string{"business_app", "workflow_template", "agent", "tool", "domain_policy"} {
		if !IsSupportedResourceType(resourceType) {
			t.Fatalf("resource type %q should be supported", resourceType)
		}
	}
	if IsSupportedResourceType("finance_report") {
		t.Fatal("business-specific resource type must not become a platform governance resource")
	}
}
