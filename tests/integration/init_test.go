package integration

import (
	"context"
	"fmt"
	"testing"

	"cmdb2neo/internal/cmdb"
	"cmdb2neo/internal/loader"
	"cmdb2neo/tests/testdata"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func TestInitNeo4jWithJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	snapshot := testdata.LoadSnapshotFromJSON(t)
	nodes, rels := cmdb.BuildInitRows(snapshot)
	ctx := context.Background()

	client, err := loader.NewClient(ctx, loader.Config{
		URI:      "bolt://localhost:7687",
		Username: "neo4j",
		Password: "StrongPassw0rd",
		Database: "neo4j",
	})
	if err != nil {
		t.Skipf("neo4j not available: %v", err)
	}
	defer client.Close(ctx)

	if err := client.RunWrite(ctx, "MATCH (n) DETACH DELETE n", nil); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	schema := loader.NewSchemaManager(client)
	if err := schema.Ensure(ctx); err != nil {
		t.Fatalf("ensure schema failed: %v", err)
	}

	nodeUpserter := loader.NewNodeUpserter(client, 200)
	if err := nodeUpserter.InitNodes(ctx, nodes); err != nil {
		t.Fatalf("init nodes failed: %v", err)
	}

	relUpserter := loader.NewRelUpserter(client, 200)
	if err := relUpserter.InitRels(ctx, rels); err != nil {
		t.Fatalf("init relationships failed: %v", err)
	}

	fixer := loader.NewEdgeFixer(client)
	if err := fixer.Run(ctx, snapshot.RunID); err != nil {
		t.Fatalf("fix edges failed: %v", err)
	}

	driver, err := neo4j.NewDriverWithContext("bolt://localhost:7687", neo4j.BasicAuth("neo4j", "StrongPassw0rd", ""))
	if err != nil {
		t.Fatalf("create driver failed: %v", err)
	}
	defer driver.Close(ctx)

	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	assertCount := func(cypher string, expected int) {
		t.Helper()
		countAny, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(ctx, cypher, nil)
			if err != nil {
				return nil, err
			}
			if !res.Next(ctx) {
				if err := res.Err(); err != nil {
					return nil, err
				}
				return int64(0), nil
			}
			val := res.Record().Values[0]
			switch v := val.(type) {
			case int64:
				return v, nil
			default:
				return nil, fmt.Errorf("unexpected type %T", val)
			}
		})
		if err != nil {
			t.Fatalf("execute %s failed: %v", cypher, err)
		}
		count := countAny.(int64)
		if int(count) != expected {
			t.Fatalf("%s expect %d got %d", cypher, expected, count)
		}
	}

	assertCount("MATCH (i:IDC) RETURN count(i)", len(snapshot.IDCs))
	assertCount("MATCH (np:NetPartition) RETURN count(np)", len(snapshot.NetworkPartitions))
	assertCount("MATCH (h:HostMachine) RETURN count(h)", len(snapshot.HostMachines))
	assertCount("MATCH (pm:PhysicalMachine) RETURN count(pm)", len(snapshot.PhysicalMachines))
	assertCount("MATCH (vm:VirtualMachine) RETURN count(vm)", len(snapshot.VirtualMachines))
	assertCount("MATCH (a:App) RETURN count(a)", len(snapshot.Apps))
}
