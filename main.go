package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

type ProcessInfo struct {
	Name string `json:"name"`
	PID  string `json:"pid"`
}

type ProcessStatus struct {
	Keyword string        `json:"keyword"`
	Running bool          `json:"running"`
	Count   int           `json:"count"`
	Matches []ProcessInfo `json:"matches,omitempty"`
}

func findProcesses(keyword string) []ProcessInfo {
	var matches []ProcessInfo

	if runtime.GOOS == "windows" {
		cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
		output, err := cmd.Output()
		if err != nil {
			return matches
		}
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, ",")
			if len(parts) >= 2 {
				imgName := strings.Trim(parts[0], "\"")
				if strings.Contains(strings.ToLower(imgName), strings.ToLower(keyword)) {
					matches = append(matches, ProcessInfo{
						Name: imgName,
						PID:  strings.Trim(parts[1], "\""),
					})
				}
			}
		}
		return matches
	}

	cmd := exec.Command("pgrep", "-fl", keyword)
	output, err := cmd.Output()
	if err != nil {
		return matches
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) >= 1 {
			pid := parts[0]
			name := ""
			if len(parts) >= 2 {
				name = parts[1]
			}
			matches = append(matches, ProcessInfo{
				Name: name,
				PID:  pid,
			})
		}
	}
	return matches
}

func processHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keyword := r.URL.Query().Get("name")
	if keyword == "" {
		http.Error(w, "missing query parameter: name", http.StatusBadRequest)
		return
	}

	matches := findProcesses(keyword)
	status := ProcessStatus{
		Keyword: keyword,
		Running: len(matches) > 0,
		Count:   len(matches),
		Matches: matches,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func main() {
	http.HandleFunc("/process/status", processHandler)

	fmt.Println("server starting on :8080")
	fmt.Println("endpoint: GET /process/status?name=<process_name>")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("server error: %v\n", err)
	}
}
