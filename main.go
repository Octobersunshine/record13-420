package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

type ProcessStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	PID     string `json:"pid,omitempty"`
}

func processExists(name string) (bool, string) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", name), "/FO", "CSV", "/NH")
		output, err := cmd.Output()
		if err != nil {
			return false, ""
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
				if strings.EqualFold(imgName, name) {
					return true, strings.Trim(parts[1], "\"")
				}
			}
		}
		return false, ""
	}
	cmd := exec.Command("pgrep", "-x", name)
	err := cmd.Run()
	if err != nil {
		return false, ""
	}
	return true, ""
}

func processHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "missing query parameter: name", http.StatusBadRequest)
		return
	}

	running, pid := processExists(name)
	status := ProcessStatus{
		Name:    name,
		Running: running,
		PID:     pid,
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
