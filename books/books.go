//go:generate sqlc generate
package books

import (
	"context"
	"database/sql"
	"embed"
	_ "embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

const Epoch = 1

var DB *sql.DB

//go:embed schema/*.sql
var migrations embed.FS

func Open(sqlite string) error {
	if DB == nil {
		sqlite_vec.Auto()
	}
	db, err := sql.Open("sqlite3", sqlite)
	if err != nil {
		return err
	}
	epoch, _ := New(db).Epoch(context.Background())
	if epoch < Epoch {
		dir, _ := migrations.ReadDir("schema")
		sort.Slice(dir, func(i, j int) bool {
			return dir[i].Name() < dir[j].Name()
		})
		for i, f := range dir[epoch:] {
			parts := strings.Split(f.Name(), "_")
			if len(parts) < 2 {
				continue
			}
			e, _ := strconv.ParseInt(parts[0], 10, 64)
			if e < epoch {
				continue
			}
			sql, _ := migrations.ReadFile("schema/" + f.Name())
			_, err := db.Exec(string(sql))
			if err != nil {
				return fmt.Errorf("%s migration failed: %w", f.Name(), err)
			}

			// first setup
			if epoch == 0 && i == 0 {
				if _, err := db.Exec("insert into migration (epoch) values (1)"); err != nil {
					return fmt.Errorf("failed to insert migration: %w", err)
				}
			} else {
				if _, err := db.Exec("update migration set epoch = ?", e); err != nil {
					return fmt.Errorf("epoch error: %w", err)
				}
			}
		}
	}
	DB = db
	return nil
}

func Session() *Queries {
	return New(DB)
}
