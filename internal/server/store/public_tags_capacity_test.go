package store

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestPublicTagsCapacityMigrationFromV5(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	files, err := fs.Glob(schemaFS, "schema/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files[:5] {
		body, err := schemaFS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO nodes (id, uuid, name, token_hash, created_at, tags, note)
		VALUES (1, 'legacy', 'legacy', 'hash', 1, 'private-tag', 'private-note');
		INSERT INTO node_info (node_id, os, updated_at) VALUES (1, 'Linux', 123);
		PRAGMA user_version = 5`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	nodes, err := s.ListNodes(context.Background())
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes: %+v %v", nodes, err)
	}
	n := nodes[0]
	if n.PublicTags != "" || n.Tags != "private-tag" || n.Note != "private-note" || n.Info.MemTotal != 0 || n.Info.DiskTotal != 0 || n.Info.OS != "Linux" || n.Info.UpdatedAt != 123 {
		t.Fatalf("migration defaults or private data: %+v", n)
	}
	n.PublicTags = "public, 公开"
	if err := s.UpdateNode(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	nodes, err = s.ListNodes(context.Background())
	if err != nil || nodes[0].PublicTags != n.PublicTags || nodes[0].Tags != "private-tag" {
		t.Fatalf("store update: %+v %v", nodes, err)
	}
}
