// Command rest is the service of Gopacabra phase 11 (P11.4): a plain REST CRUD
// service on fiber v3 over SQLite through modernc.org/sqlite, a pure-Go
// driver, so the gc build needs no cgo (CGO_ENABLED=0).
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gofiber/fiber/v3"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	_ "modernc.org/sqlite"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("rest", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	path := fs.String("db", "rest.sqlite", "SQLite database file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := openDB(ctx, *path)
	if err != nil {
		return err
	}
	err = newApp(db).Listen(*addr, fiber.ListenConfig{GracefulContext: ctx, DisableStartupMessage: true})
	return errors.Join(err, db.Close())
}

// pools holds the two connection pools of one SQLite database. SQLite has a
// single write lock: writers on several connections starve in its busy
// handler and fail with SQLITE_BUSY under load, so every write goes through
// the one connection of write and queues in database/sql; reads run on their
// own pool beside it (WAL).
type pools struct {
	read, write *sql.DB
}

func (p pools) Close() error { return errors.Join(p.read.Close(), p.write.Close()) }

func openDB(ctx context.Context, path string) (pools, error) {
	// synchronous(NORMAL), the usual WAL setting, syncs at checkpoints rather
	// than on every commit, so the disk does not bound the write rate.
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
	write, err := sql.Open("sqlite", dsn)
	if err != nil {
		return pools{}, fmt.Errorf("open %s: %w", path, err)
	}
	write.SetMaxOpenConns(1)
	_, err = write.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS items (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		qty INTEGER NOT NULL
	)`)
	if err != nil {
		return pools{}, errors.Join(fmt.Errorf("create schema in %s: %w", path, err), write.Close())
	}
	read, err := sql.Open("sqlite", dsn+"&_pragma=query_only(1)")
	if err != nil {
		return pools{}, errors.Join(fmt.Errorf("open %s: %w", path, err), write.Close())
	}
	read.SetMaxOpenConns(8)
	read.SetMaxIdleConns(8)
	return pools{read: read, write: write}, nil
}

type item struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Qty  int64  `json:"qty"`
}

func newApp(db pools) *fiber.App {
	app := fiber.New(fiber.Config{
		// The default handler writes err.Error() to the client.
		ErrorHandler: func(c fiber.Ctx, err error) error {
			if e, ok := errors.AsType[*fiber.Error](err); ok {
				return c.Status(e.Code).SendString(e.Message)
			}
			log.Printf("%s %s: %v", c.Method(), c.Path(), err)
			return c.Status(fiber.StatusInternalServerError).SendString("internal error")
		},
	})
	app.Use(recoverer.New())

	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	app.Post("/items", func(c fiber.Ctx) error {
		it, err := bindItem(c)
		if err != nil {
			return err
		}
		err = db.write.QueryRowContext(c.Context(),
			`INSERT INTO items (name, qty) VALUES (?, ?) RETURNING id`, it.Name, it.Qty).Scan(&it.ID)
		if err != nil {
			return fmt.Errorf("insert item: %w", err)
		}
		return c.Status(fiber.StatusCreated).JSON(it)
	})

	app.Get("/items/:id", func(c fiber.Ctx) error {
		id, err := pathID(c)
		if err != nil {
			return err
		}
		it := item{ID: id}
		err = db.read.QueryRowContext(c.Context(), `SELECT name, qty FROM items WHERE id = ?`, id).Scan(&it.Name, &it.Qty)
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("get item %d: %w", id, err)
		}
		return c.JSON(it)
	})

	app.Put("/items/:id", func(c fiber.Ctx) error {
		id, err := pathID(c)
		if err != nil {
			return err
		}
		it, err := bindItem(c)
		if err != nil {
			return err
		}
		err = db.write.QueryRowContext(c.Context(),
			`UPDATE items SET name = ?, qty = ? WHERE id = ? RETURNING id`, it.Name, it.Qty, id).Scan(&it.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("update item %d: %w", id, err)
		}
		return c.JSON(it)
	})

	app.Delete("/items/:id", func(c fiber.Ctx) error {
		id, err := pathID(c)
		if err != nil {
			return err
		}
		res, err := db.write.ExecContext(c.Context(), `DELETE FROM items WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("delete item %d: %w", id, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete item %d: %w", id, err)
		}
		if n == 0 {
			return fiber.ErrNotFound
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	app.Get("/items", func(c fiber.Ctx) error {
		after, err := queryInt(c, "after", 0)
		if err != nil {
			return err
		}
		limit, err := queryInt(c, "limit", 100)
		if err != nil {
			return err
		}
		if limit < 1 || limit > 1000 {
			return fiber.NewError(fiber.StatusBadRequest, "limit must be in [1, 1000]")
		}
		rows, err := db.read.QueryContext(c.Context(),
			`SELECT id, name, qty FROM items WHERE id > ? ORDER BY id LIMIT ?`, after, limit)
		if err != nil {
			return fmt.Errorf("list items: %w", err)
		}
		defer rows.Close()
		items := make([]item, 0, limit) // fiber encodes with encoding/json, which writes a nil slice as null
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.ID, &it.Name, &it.Qty); err != nil {
				return fmt.Errorf("scan item: %w", err)
			}
			items = append(items, it)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("list items: %w", err)
		}
		return c.JSON(items)
	})

	return app
}

// bindItem decodes and validates the item body of POST and PUT.
func bindItem(c fiber.Ctx) (item, error) {
	var it item
	if err := c.Bind().Body(&it); err != nil {
		return item{}, fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if it.Name == "" {
		return item{}, fiber.NewError(fiber.StatusBadRequest, "name is required")
	}
	if it.Qty < 0 {
		return item{}, fiber.NewError(fiber.StatusBadRequest, "qty must not be negative")
	}
	return it, nil
}

// pathID reads the :id route parameter of /items/:id.
func pathID(c fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, "id must be an integer")
	}
	return id, nil
}

// queryInt reads an integer query parameter; an absent one is def.
func queryInt(c fiber.Ctx, key string, def int64) (int64, error) {
	s := c.Query(key)
	if s == "" {
		return def, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, key+" must be an integer")
	}
	return v, nil
}
