package catalog

import "testing"

func TestWithoutCustom(t *testing.T) {
	sel := []Entry{
		{ID: "PX3013", Custom: false},
		{ID: "PX2101", Custom: true},
		{ID: "PX3004", Custom: false},
		{ID: "PX2106", Custom: true},
	}
	got := WithoutCustom(sel)
	if len(got) != 2 {
		t.Fatalf("WithoutCustom kept %d, want 2", len(got))
	}
	for _, e := range got {
		if e.Custom {
			t.Errorf("WithoutCustom kept a custom check: %s", e.ID)
		}
	}
	if got[0].ID != "PX3013" || got[1].ID != "PX3004" {
		t.Errorf("order not preserved: %s, %s", got[0].ID, got[1].ID)
	}
	// AnyCustom must now be false on the filtered set.
	if AnyCustom(got) {
		t.Error("AnyCustom(WithoutCustom(sel)) = true")
	}
}
