package collector

import (
	"dameng_exporter/config"
	"database/sql/driver"
	"errors"
	"math"
	"testing"
)

// Query/scan/EOF/cache behavior also compares complete metric payloads with the
// committed pre-change baseline in TestBusinessCollectionCompatibility.
func TestUnserializableTableResultNeverCached(t *testing.T) {
	setupCollectionLogTest(t)
	for _, f := range businessFixtures()[:2] {
		resetFixtureCache(t, f)
		row := append([]driver.Value(nil), f.row...)
		row[2] = math.NaN()
		conn, _ := fixtureDB(t, f, [][]driver.Value{row}, nil, nil, nil)
		facts, _ := collectTableFixture(t, f, conn)
		if !facts.Issue {
			t.Fatal("marshal error missing from facts")
		}
		if _, ok := config.GetFromCache(f.cacheKey + "_fixture"); ok {
			t.Fatal("invalid value cached")
		}
	}
}
func TestTableScanAndRowErrorsRemainIssues(t *testing.T) {
	setupCollectionLogTest(t)
	for _, f := range businessFixtures()[:2] {
		for _, scan := range []bool{true, false} {
			resetFixtureCache(t, f)
			row := append([]driver.Value(nil), f.row...)
			var readErr error
			if scan {
				row[1] = "bad number"
			} else {
				readErr = errors.New("read failed")
			}
			conn, _ := fixtureDB(t, f, [][]driver.Value{row}, nil, readErr, nil)
			facts, _ := collectTableFixture(t, f, conn)
			if !facts.Issue {
				t.Fatal("incomplete result missing issue")
			}
			if _, ok := config.GetFromCache(f.cacheKey + "_fixture"); ok {
				t.Fatal("incomplete rows cached")
			}
		}
	}
}
