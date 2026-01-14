package completion

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

//go:embed data/commands.json
var commandFS embed.FS

// Database manages the command database with thread-safe access
type Database struct {
	commands map[string]*Command // Indexed by command name
	mu       sync.RWMutex
}

// NewDatabase creates and initializes a new command database
func NewDatabase() (*Database, error) {
	db := &Database{
		commands: make(map[string]*Command),
	}

	if err := db.load(); err != nil {
		return nil, err
	}

	return db, nil
}

// load reads and parses the commands.json file
func (db *Database) load() error {
	data, err := commandFS.ReadFile("data/commands.json")
	if err != nil {
		return fmt.Errorf("failed to read commands.json: %w", err)
	}

	var commands []Command
	if err := json.Unmarshal(data, &commands); err != nil {
		return fmt.Errorf("failed to parse commands.json: %w", err)
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	for i := range commands {
		cmd := &commands[i]
		db.commands[cmd.Name] = cmd

		// Index aliases
		for _, alias := range cmd.Aliases {
			db.commands[alias] = cmd
		}
	}

	return nil
}

// FindCommands finds commands matching prefix
func (db *Database) FindCommands(prefix string) []*Command {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var results []*Command
	for name, cmd := range db.commands {
		if strings.HasPrefix(name, prefix) {
			results = append(results, cmd)
		}
	}

	// Sort by relevance (exact match first, then alphabetically)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Name == prefix {
			return true
		}
		if results[j].Name == prefix {
			return false
		}
		return results[i].Name < results[j].Name
	})

	return results
}

// GetCommand retrieves a specific command by name
func (db *Database) GetCommand(name string) (*Command, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	cmd, ok := db.commands[name]
	return cmd, ok
}

// GetAllCommands returns all commands in the database
func (db *Database) GetAllCommands() []*Command {
	db.mu.RLock()
	defer db.mu.RUnlock()

	results := make([]*Command, 0, len(db.commands))
	seen := make(map[string]bool)

	for _, cmd := range db.commands {
		// Deduplicate (aliases point to same command)
		if !seen[cmd.Name] {
			results = append(results, cmd)
			seen[cmd.Name] = true
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	return results
}
