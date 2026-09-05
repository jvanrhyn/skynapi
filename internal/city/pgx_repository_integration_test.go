package city

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// Run against a migrated disposable database using CITY_TEST_DATABASE_URL.
// The search fixtures live in a connection-local temporary table; existing
// all_countries data is never changed. country_codes is read-only.
func TestSearchPostgres(t *testing.T) {
	url := os.Getenv("CITY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set CITY_TEST_DATABASE_URL to run PostgreSQL search tests")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	require.NoError(t, err)
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TEMP TABLE all_countries (LIKE public.all_countries INCLUDING ALL);
 INSERT INTO all_countries (geonameid,name,asciiname,alternatenames,latitude,longitude,feature_class,feature_code,country_code,population,timezone) VALUES
 (1,'Sandton','Sandton','sandtwn',-26.104,28.054,'P','PPLX','ZA',0,'Africa/Johannesburg'),
 (2,'Sandton Heights','Sandton Heights','',-26,28,'P','PPL','ZA',1000000,'Africa/Johannesburg'),
 (3,'Sandton Station','Sandton Station','',-26,28,'S','RSTN','ZA',10,'Africa/Johannesburg'),
 (4,'Sandton ruins','Sandton ruins','',-26,28,'P','PPLQ','ZA',20,'Africa/Johannesburg'),
 (5,'Fourways','Fourways','Four Ways',-26.02111,28.00917,'P','PPL','ZA',NULL,'Africa/Johannesburg'),
 (6,'Paris','Paris','',48.85,2.35,'P','PPLC','FR',2100000,'Europe/Paris'),
 (7,'Paris','Paris','',33.66,-95.55,'P','PPL','US',24000,'America/Chicago');`)
	require.NoError(t, err)
	repo := NewRepository(pool)
	t.Run("zero population exact suburb outranks larger fuzzy match", func(t *testing.T) {
		cities, total, err := repo.Search(ctx, SearchParams{Q: "Sandton", Page: 1, Limit: 20})
		require.NoError(t, err)
		require.Equal(t, 2, total)
		require.Len(t, cities, 2)
		require.Equal(t, "Sandton", cities[0].Name)
		require.Equal(t, "South Africa", cities[0].CountryName)
		require.Equal(t, -26.104, cities[0].Lat)
		require.Equal(t, "Africa/Johannesburg", cities[0].Timezone)
	})
	t.Run("alternative name and null population", func(t *testing.T) {
		cities, _, err := repo.Search(ctx, SearchParams{Q: "four ways", Page: 1, Limit: 20})
		require.NoError(t, err)
		require.NotEmpty(t, cities)
		require.Equal(t, "Fourways", cities[0].Name)
	})
	t.Run("population resolves equal names", func(t *testing.T) {
		cities, _, err := repo.Search(ctx, SearchParams{Q: "Paris", Page: 1, Limit: 20})
		require.NoError(t, err)
		require.Len(t, cities, 2)
		require.Equal(t, "FR", cities[0].CountryCode)
	})
	t.Run("wildcards remain literal", func(t *testing.T) {
		cities, total, err := repo.Search(ctx, SearchParams{Q: "%_", Page: 1, Limit: 20})
		require.NoError(t, err)
		require.Zero(t, total)
		require.Empty(t, cities)
	})
}

func TestRefreshedSouthAfrica(t *testing.T) {
	url := os.Getenv("CITY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set CITY_TEST_DATABASE_URL to a database with migration 010")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	defer pool.Close()
	repo := NewRepository(pool)
	for _, name := range []string{"Sandton", "Fourways", "Bryanston"} {
		t.Run(name, func(t *testing.T) {
			cities, _, err := repo.Search(ctx, SearchParams{Q: name, Page: 1, Limit: 20})
			require.NoError(t, err)
			require.NotEmpty(t, cities)
			require.Equal(t, name, cities[0].Name)
			require.Equal(t, "ZA", cities[0].CountryCode)
			require.Equal(t, "Africa/Johannesburg", cities[0].Timezone)
		})
	}
}
