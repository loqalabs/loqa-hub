/*
 * This file is part of Loqa (https://github.com/loqalabs/loqa).
 * Copyright (C) 2025 Loqa Labs
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program. If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	defaultHubURL = "http://localhost:3000"
)

type SkillInfo struct {
	Manifest   SkillManifest `json:"manifest"`
	Config     SkillConfig   `json:"config"`
	Status     SkillStatus   `json:"status"`
	LoadedAt   time.Time     `json:"loaded_at"`
	LastUsed   *time.Time    `json:"last_used,omitempty"`
	ErrorCount int           `json:"error_count"`
	LastError  string        `json:"last_error,omitempty"`
	PluginPath string        `json:"plugin_path"`
}

type SkillManifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	License     string `json:"license"`
}

type SkillConfig struct {
	SkillID    string                 `json:"skill_id"`
	Name       string                 `json:"name"`
	Version    string                 `json:"version"`
	Config     map[string]interface{} `json:"config"`
	Enabled    bool                   `json:"enabled"`
	Timeout    string                 `json:"timeout"`
	MaxRetries int                    `json:"max_retries"`
}

type SkillStatus struct {
	State      string    `json:"state"`
	Healthy    bool      `json:"healthy"`
	LastError  string    `json:"last_error,omitempty"`
	LastUsed   time.Time `json:"last_used"`
	UsageCount int64     `json:"usage_count"`
}

func main() {
	var (
		hubURL    = flag.String("hub", defaultHubURL, "URL of the Loqa hub")
		action    = flag.String("action", "list", "Action to perform: list, load, unload, enable, disable, reload, info")
		skillID   = flag.String("skill", "", "Skill ID for actions")
		skillPath = flag.String("path", "", "Path to skill directory for load action")
		verbose   = flag.Bool("v", false, "Verbose output")
		format    = flag.String("format", "table", "Output format: table, json")
	)
	flag.Parse()

	client := &SkillCLI{
		hubURL:  *hubURL,
		verbose: *verbose,
		format:  *format,
	}

	switch *action {
	case "list":
		err := client.listSkills()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "info":
		if *skillID == "" {
			fmt.Fprintf(os.Stderr, "Error: skill ID required for info action\n")
			os.Exit(1)
		}
		err := client.getSkill(*skillID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "load":
		if *skillPath == "" {
			fmt.Fprintf(os.Stderr, "Error: skill path required for load action\n")
			os.Exit(1)
		}
		err := client.loadSkill(*skillPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "unload":
		if *skillID == "" {
			fmt.Fprintf(os.Stderr, "Error: skill ID required for unload action\n")
			os.Exit(1)
		}
		err := client.unloadSkill(*skillID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "enable":
		if *skillID == "" {
			fmt.Fprintf(os.Stderr, "Error: skill ID required for enable action\n")
			os.Exit(1)
		}
		err := client.enableSkill(*skillID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "disable":
		if *skillID == "" {
			fmt.Fprintf(os.Stderr, "Error: skill ID required for disable action\n")
			os.Exit(1)
		}
		err := client.disableSkill(*skillID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "reload":
		if *skillID == "" {
			fmt.Fprintf(os.Stderr, "Error: skill ID required for reload action\n")
			os.Exit(1)
		}
		err := client.reloadSkill(*skillID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown action %s\n", *action)
		fmt.Fprintf(os.Stderr, "Valid actions: list, load, unload, enable, disable, reload, info\n")
		os.Exit(1)
	}
}

type SkillCLI struct {
	hubURL  string
	verbose bool
	format  string
}

func (c *SkillCLI) listSkills() error {
	resp, err := http.Get(c.hubURL + "/api/skills")
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Skills []SkillInfo `json:"skills"`
		Count  int         `json:"count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if c.format == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result.Skills)
	}

	// Table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tVERSION\tSTATUS\tENABLED\tERRORS\tLAST USED")
	fmt.Fprintln(w, "---\t----\t-------\t------\t-------\t------\t---------")

	for _, skill := range result.Skills {
		lastUsed := "never"
		if skill.LastUsed != nil {
			lastUsed = skill.LastUsed.Format("2006-01-02 15:04")
		}

		enabled := "✓"
		if !skill.Config.Enabled {
			enabled = "✗"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
			skill.Manifest.ID,
			skill.Manifest.Name,
			skill.Manifest.Version,
			skill.Status.State,
			enabled,
			skill.ErrorCount,
			lastUsed,
		)
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("error flushing output: %w", err)
	}
	fmt.Printf("\nTotal: %d skills\n", result.Count)
	return nil
}

func (c *SkillCLI) getSkill(skillID string) error {
	resp, err := http.Get(c.hubURL + "/api/skills/" + skillID)
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("skill %s not found", skillID)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var skill SkillInfo
	if err := json.NewDecoder(resp.Body).Decode(&skill); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if c.format == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(skill)
	}

	// Detailed format
	fmt.Printf("Skill Information:\n")
	fmt.Printf("  ID:          %s\n", skill.Manifest.ID)
	fmt.Printf("  Name:        %s\n", skill.Manifest.Name)
	fmt.Printf("  Version:     %s\n", skill.Manifest.Version)
	fmt.Printf("  Description: %s\n", skill.Manifest.Description)
	fmt.Printf("  Author:      %s\n", skill.Manifest.Author)
	fmt.Printf("  License:     %s\n", skill.Manifest.License)
	fmt.Printf("\nStatus:\n")
	fmt.Printf("  State:       %s\n", skill.Status.State)
	fmt.Printf("  Healthy:     %s\n", formatBool(skill.Status.Healthy))
	fmt.Printf("  Enabled:     %s\n", formatBool(skill.Config.Enabled))
	fmt.Printf("  Loaded At:   %s\n", skill.LoadedAt.Format("2006-01-02 15:04:05"))

	if skill.LastUsed != nil {
		fmt.Printf("  Last Used:   %s\n", skill.LastUsed.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("  Usage Count: %d\n", skill.Status.UsageCount)
	fmt.Printf("  Error Count: %d\n", skill.ErrorCount)

	if skill.LastError != "" {
		fmt.Printf("  Last Error:  %s\n", skill.LastError)
	}

	fmt.Printf("\nConfiguration:\n")
	fmt.Printf("  Timeout:     %s\n", skill.Config.Timeout)
	fmt.Printf("  Max Retries: %d\n", skill.Config.MaxRetries)
	fmt.Printf("  Plugin Path: %s\n", skill.PluginPath)

	if len(skill.Config.Config) > 0 {
		fmt.Printf("\nCustom Config:\n")
		for key, value := range skill.Config.Config {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}

	return nil
}

func (c *SkillCLI) loadSkill(skillPath string) error {
	payload := map[string]string{
		"skill_path": skillPath,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(c.hubURL+"/api/skills", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("skill already loaded")
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("Skill loaded successfully from %s\n", skillPath)
	return nil
}

func (c *SkillCLI) unloadSkill(skillID string) error {
	req, err := http.NewRequest(http.MethodDelete, c.hubURL+"/api/skills/"+skillID, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("skill %s not found", skillID)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	fmt.Printf("Skill %s unloaded successfully\n", skillID)
	return nil
}

func (c *SkillCLI) enableSkill(skillID string) error {
	return c.skillAction(skillID, "enable")
}

func (c *SkillCLI) disableSkill(skillID string) error {
	return c.skillAction(skillID, "disable")
}

func (c *SkillCLI) reloadSkill(skillID string) error {
	return c.skillAction(skillID, "reload")
}

func (c *SkillCLI) skillAction(skillID, action string) error {
	resp, err := http.Post(c.hubURL+"/api/skills/"+skillID+"/"+action, "application/json", strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("skill %s not found", skillID)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("Skill %s %sd successfully\n", skillID, action)
	return nil
}

func formatBool(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}
