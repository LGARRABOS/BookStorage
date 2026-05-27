package server

import (
	"database/sql"
	"testing"
)

func TestEffectiveLinkDotStatus_onlyInProgress(t *testing.T) {
	deadProbe := sql.NullString{String: "down", Valid: true}
	wReading := workRow{
		Status:          sql.NullString{String: "En cours", Valid: true},
		Link:            sql.NullString{String: "https://example.com/a", Valid: true},
		LinkProbeStatus: deadProbe,
	}
	if st := effectiveLinkDotStatus(wReading, nil); st != "down" {
		t.Fatalf("expected down for En cours, got %q", st)
	}

	wDone := workRow{
		Status:          sql.NullString{String: "Terminé", Valid: true},
		Link:            sql.NullString{String: "https://example.com/a", Valid: true},
		LinkProbeStatus: deadProbe,
	}
	if st := effectiveLinkDotStatus(wDone, nil); st != "none" {
		t.Fatalf("expected none for Terminé, got %q", st)
	}
}
