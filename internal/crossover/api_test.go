package crossover

import (
	"context"
	"testing"

	"github.com/jxsl13/perfscan/config"
)

func TestMissingModelFactoryReturnsErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if _, err := RecordCampaign(ctx, &Options{}, &config.DispatchCrossoverContract{}, nil, nil); err == nil {
		t.Fatal("accepted missing record factory")
	}
	if _, err := CurrentBuild(ctx, ".", ".", "", &config.DispatchCrossoverContract{}, nil); err == nil {
		t.Fatal("accepted missing current factory")
	}
	if _, err := VerifyCampaign(ctx, ".", ".", "go", "", "", nil); err == nil {
		t.Fatal("accepted missing verify factory")
	}
}
