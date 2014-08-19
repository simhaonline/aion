package cql_test

import (
	"math"
	"testing"
	"time"

	"github.com/FlukeNetworks/aion"
	"github.com/FlukeNetworks/aion/aiontest"
	"github.com/FlukeNetworks/aion/cql"
	"github.com/gocql/gocql"
)

func newCQLTestSession() (*gocql.Session, error) {
	cluster := gocql.NewCluster("172.28.128.2")
	cluster.Keyspace = "timedb"
	return cluster.CreateSession()
}

func TestCQLCache(t *testing.T) {
	session, err := newCQLTestSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	cache := cql.CQLCache{
		ColumnFamily: "cache",
		Session:      session,
	}
	filter := aion.NewAggregateFilter(0, []string{"raw"}, nil)
	level := aion.Level{
		Filter: filter,
		Store:  &cache,
	}
	aiontest.TestLevel(&level, t, time.Second, 60*time.Second)
}

func TestCQLStore(t *testing.T) {
	session, err := newCQLTestSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	store := aion.NewBucketStore(60*time.Second, math.Pow10(1))
	repo := cql.Repository{
		ColumnFamily: "buckets",
		Session:      session,
	}
	store.Repository = repo
	level := aion.Level{
		Filter: aion.NewAggregateFilter(0, []string{"raw"}, nil),
		Store:  store,
	}
	aiontest.TestLevel(&level, t, time.Second, store.Duration)
}