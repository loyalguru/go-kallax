// Package generator implements the processor of source code and generator of
// kallax models based on Go source code.
package generator // import "github.com/loyalguru/go-kallax/generator"

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/fatih/color"
)

// Generator is in charge of generating files for packages.
type Generator struct {
	filename string
}

// NewGenerator creates a new generator that can save on the given filename.
func NewGenerator(filename string) *Generator {
	return &Generator{filename}
}

// Generate writes the file with the contents of the given package.
func (g *Generator) Generate(pkg *Package) error {
	return g.writeFile(pkg)
}

func (g *Generator) writeFile(pkg *Package) (err error) {
	file, err := os.Create(g.filename)
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("kallax: PANIC during '%s' generation:\n%s\n\n", g.filename, r)
			if err == nil {
				err = fmt.Errorf(string(debug.Stack()))
			}
		}

		file.Close()
		if err != nil {
			if os.Remove(g.filename) == nil {
				fmt.Println("kallax: No file generated due to an occurred error:")
			} else {
				fmt.Printf("kallax: The autogenerated file '%s' could not be completed nor deleted due to an occurred error:\n", g.filename)
			}
		}
	}()

	return Base.Execute(file, pkg)
}

// Timestamper is a function that returns the current time.
type Timestamper func() time.Time

// MigrationGenerator is a generator of migrations.
type MigrationGenerator struct {
	name string
	dir  string
	now  Timestamper
}

type migrationFileType string

const (
	migrationUp   = migrationFileType("up.sql")
	migrationDown = migrationFileType("down.sql")
	migrationLock = migrationFileType("lock.json")
)

// NewMigrationGenerator returns a new migration generator with the given
// migrations directory.
func NewMigrationGenerator(name, dir string) *MigrationGenerator {
	return &MigrationGenerator{slugify(name), dir, time.Now}
}

// Build creates a new migration from a set of scanned packages.
func (g *MigrationGenerator) Build(pkgs ...*Package) (*Migration, error) {
	old, err := g.LoadLock()
	if err != nil {
		return nil, err
	}

	new, err := SchemaFromPackages(pkgs...)
	if err != nil {
		return nil, err
	}

	return NewMigration(old, new)
}

// Generate will generate the given migration.
func (g *MigrationGenerator) Generate(migration *Migration) error {
	g.printMigrationInfo(migration)
	if len(migration.Up) == 0 {
		return nil
	}
	return g.writeMigration(migration)
}

func (g *MigrationGenerator) printMigrationInfo(migration *Migration) {
	if len(migration.Up) == 0 {
		fmt.Println("There are no changes since last migration. Nothing will be generated.")
		return
	}

	fmt.Println("There are changes since last migration.\n\nThese are the proposed changes:")
	for _, change := range migration.Up {
		c := color.FgGreen
		switch change.(type) {
		case *DropColumn, *DropTable:
			c = color.FgRed
		case *ManualChange:
			c = color.FgYellow
		}
		color := color.New(c, color.Bold)
		color.Printf(" => ")
		fmt.Println(change.String())
	}
}

// LoadLock loads the lock file.
func (g *MigrationGenerator) LoadLock() (*DBSchema, error) {
	bytes, err := ioutil.ReadFile(filepath.Join(g.dir, string(migrationLock)))
	if os.IsNotExist(err) {
		return new(DBSchema), nil
	} else if err != nil {
		return nil, fmt.Errorf("error opening lock file: %s", err)
	}

	var schema DBSchema
	if err := json.Unmarshal(bytes, &schema); err != nil {
		return nil, fmt.Errorf("error unmarshaling lock schema: %s", err)
	}

	return &schema, nil
}

func (g *MigrationGenerator) writeMigration(migration *Migration) error {
	t := g.now()
	files := []struct {
		file    string
		content encoding.TextMarshaler
	}{
		{filepath.Join(g.dir, string(migrationLock)), migration.Lock},
		{g.migrationFile(migrationDown, t), migration.Down},
		{g.migrationFile(migrationUp, t), migration.Up},
	}

	for _, f := range files {
		if err := g.createFile(f.file, f.content); err != nil {
			return err
		}
	}

	return nil
}

func (g *MigrationGenerator) migrationFile(typ migrationFileType, t time.Time) string {
	return filepath.Join(g.dir, fmt.Sprintf("%d_%s.%s", t.Unix(), g.name, typ))
}

func (g *MigrationGenerator) createFile(filename string, marshaler encoding.TextMarshaler) error {
	data, err := marshaler.MarshalText()
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filename, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("error opening file: %s: %s", filename, err)
	}

	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("error writing file: %s: %s", filename, err)
	}

	return nil
}

func slugify(str string) string {
	var buf bytes.Buffer
	for _, r := range strings.ToLower(str) {
		if ('a' <= r && r <= 'z') || ('0' <= r && r <= '9') {
			buf.WriteRune(r)
		} else if r == ' ' || r == '_' || r == '-' {
			buf.WriteRune('_')
		}
	}
	return buf.String()
}
