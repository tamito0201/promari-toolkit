package learn

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
)

// TestOutOfDistributionSeenBits: the bitset was sized seen_bits/64 words, so a
// seen_bits that is not a multiple of 64 dropped the last partial word and
// training panicked with an index out of range (and 32 gave zero words).
func TestOutOfDistributionSeenBits(t *testing.T) {
	tests := []struct {
		name      string
		bits      int
		wantWords int
	}{
		{name: "a multiple of 64", bits: 65536, wantWords: 1024},
		{name: "not a multiple of 64", bits: 100, wantWords: 2},
		{name: "less than one word", bits: 32, wantWords: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := model.Settings{Features: model.FeatureSpec{NGramMin: 1, NGramMax: 3, HashBuckets: 64, SeenBits: tt.bits}}
			data := make([]prepared, 0, 50)
			for i := range 50 {
				data = append(data, prepared{sig: service.Signals{Normalised: fmt.Sprintf("テスト用の文 %d number", i)}})
			}
			var art model.Artifact
			trainer{st: st, data: data}.outOfDistribution(&art)
			art.Features = st.Features
			// Every n-gram seen in training reads back as seen.
			unseen := service.UnseenRatio(service.Signals{Normalised: data[7].sig.Normalised}, art)
			if diff := cmp.Diff([]any{tt.wantWords, 0.0}, []any{len(art.Seen), unseen}); diff != "" {
				t.Errorf("words and unseen ratio (-want +got):\n%s", diff)
			}
		})
	}
}
