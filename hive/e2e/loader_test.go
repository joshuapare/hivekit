package e2e

import (
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

var td = path.Join("..", "..", "testdata")

var testHives = []string{
	"large",
	"minimal",
	"rlenvalue_test_hive",
	"typed_values",
}

var integrationHives = []string{
	"suite/windows-2003-server-software",
	"suite/windows-2003-server-system",
	"suite/windows-2012-software",
	"suite/windows-2012-system",
	"suite/windows-8-consumer-preview-software",
	"suite/windows-8-consumer-preview-system",
	"suite/windows-8-enterprise-software",
	"suite/windows-8-enterprise-system",
	"suite/windows-xp-2-software",
	"suite/windows-xp-2-system",
	"suite/windows-xp-software",
	"suite/windows-xp-system",
}

var allTestHives = slices.Concat(testHives, integrationHives)

func TestIntegration_OpenRealHivesIfPresent(t *testing.T) {
	for _, name := range allTestHives {
		p := filepath.Join(td, name)
		t.Run(name, func(t *testing.T) {
			_, err := os.Stat(p)
			if os.IsNotExist(err) {
				t.Skipf("hive %s not present", p)
			}
			require.NoError(t, err)

			h, err := hive.Open(p)
			require.NoError(t, err, "Open(%s)", p)
			t.Cleanup(func() { _ = h.Close() })

			data := h.Bytes()
			require.GreaterOrEqual(t, len(data), format.HeaderSize)
			require.Equal(t,
				string(format.REGFSignature),
				string(data[0:4]),
			)

			// walk HBINs
			it, err := h.HBINs()
			require.NoError(t, err)

			count := 0
			for {
				hb, iterErr := it.Next()
				if errors.Is(iterErr, io.EOF) {
					break
				}
				require.NoError(t, iterErr)

				// header
				require.Equal(t, string(format.HBINSignature), string(hb.Data[0:4]))
				require.NotZero(t, hb.Size)
				require.LessOrEqual(t, int(hb.Offset+hb.Size), len(data))

				count++
			}

			require.Positive(t, count, "expected at least one HBIN in %s", name)
		})
	}
}
