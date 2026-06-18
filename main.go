package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type ProcessInfo struct {
	Name       string  `json:"name"`
	PID        string  `json:"pid"`
	CPUPercent float64 `json:"cpu_percent"`
	MemKB      uint64  `json:"mem_kb"`
	Alarm      bool    `json:"alarm"`
}

type ProcessStatus struct {
	Keyword      string        `json:"keyword"`
	Running      bool          `json:"running"`
	Count        int           `json:"count"`
	AlarmCount   int           `json:"alarm_count"`
	CPUThreshold float64       `json:"cpu_threshold,omitempty"`
	MemThreshold uint64        `json:"mem_threshold,omitempty"`
	Matches      []ProcessInfo `json:"matches,omitempty"`
}

func findProcesses(keyword string, cpuThreshold float64, memThreshold uint64) ([]ProcessInfo, error) {
	var matches []ProcessInfo

	if runtime.GOOS == "windows" {
		cmd := exec.Command("wmic", "path", "Win32_PerfFormattedData_PerfProc_Process",
			"get", "Name,IDProcess,PercentProcessorTime,WorkingSet", "/format:csv")
		output, err := cmd.Output()
		if err != nil {
			return matches, err
		}

		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) < 2 {
			return matches, nil
		}

		headerIdx := -1
		var colIndex map[string]int
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(strings.ToLower(line), "idprocess") {
				headers := strings.Split(line, ",")
				colIndex = make(map[string]int)
				for j, h := range headers {
					colIndex[strings.TrimSpace(strings.ToLower(h))] = j
				}
				headerIdx = i
				break
			}
		}
		if headerIdx < 0 {
			return matches, nil
		}

		for _, line := range lines[headerIdx+1:] {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Split(line, ",")
			nameIdx := colIndex["name"]
			pidIdx := colIndex["idprocess"]
			cpuIdx := colIndex["percentprocessortime"]
			memIdx := colIndex["workingset"]

			if nameIdx < 0 || pidIdx < 0 || cpuIdx < 0 || memIdx < 0 {
				continue
			}
			if len(fields) <= max(nameIdx, pidIdx, cpuIdx, memIdx) {
				continue
			}

			name := strings.TrimSpace(fields[nameIdx])
			if strings.Contains(strings.ToLower(name), strings.ToLower(keyword)) {
				pid := strings.TrimSpace(fields[pidIdx])
				cpuStr := strings.TrimSpace(fields[cpuIdx])
				memStr := strings.TrimSpace(fields[memIdx])

				cpu, _ := strconv.ParseFloat(cpuStr, 64)
				memBytes, _ := strconv.ParseUint(memStr, 10, 64)
				memKB := memBytes / 1024

				alarm := false
				if cpuThreshold > 0 && cpu > cpuThreshold {
					alarm = true
				}
				if memThreshold > 0 && memKB > memThreshold {
					alarm = true
				}

				matches = append(matches, ProcessInfo{
					Name:       name,
					PID:        pid,
					CPUPercent: cpu,
					MemKB:      memKB,
					Alarm:      alarm,
				})
			}
		}
		return matches, nil
	}

	cmd := exec.Command("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		return matches, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return matches, nil
	}

	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}

		cpu, _ := strconv.ParseFloat(fields[2], 64)
		mem, _ := strconv.ParseFloat(fields[3], 64)
		pid := fields[1]
		cmd := strings.Join(fields[10:], " ")

		if strings.Contains(strings.ToLower(cmd), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(fields[10]), strings.ToLower(keyword)) {
			memKB := uint64(mem * 1024)

			alarm := false
			if cpuThreshold > 0 && cpu > cpuThreshold {
				alarm = true
			}
			if memThreshold > 0 && memKB > memThreshold {
				alarm = true
			}

			matches = append(matches, ProcessInfo{
				Name:       fields[10],
				PID:        pid,
				CPUPercent: cpu,
				MemKB:      memKB,
				Alarm:      alarm,
			})
		}
	}
	return matches, nil
}

func max(a, b, c, d int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	if d > m {
		m = d
	}
	return m
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

	var cpuThreshold float64
	if cpuStr := r.URL.Query().Get("cpu"); cpuStr != "" {
		if v, err := strconv.ParseFloat(cpuStr, 64); err == nil {
			cpuThreshold = v
		}
	}

	var memThreshold uint64
	if memStr := r.URL.Query().Get("mem"); memStr != "" {
		if v, err := strconv.ParseUint(memStr, 10, 64); err == nil {
			memThreshold = v
		}
	}

	matches, err := findProcesses(keyword, cpuThreshold, memThreshold)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to query processes: %v", err), http.StatusInternalServerError)
		return
	}

	alarmCount := 0
	for _, p := range matches {
		if p.Alarm {
			alarmCount++
		}
	}

	status := ProcessStatus{
		Keyword:      keyword,
		Running:      len(matches) > 0,
		Count:        len(matches),
		AlarmCount:   alarmCount,
		CPUThreshold: cpuThreshold,
		MemThreshold: memThreshold,
		Matches:      matches,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func main() {
	http.HandleFunc("/process/status", processHandler)

	fmt.Println("server starting on :8080")
	fmt.Println("endpoint: GET /process/status?name=<keyword>&cpu=<threshold>&mem=<threshold_kb>")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("server error: %v\n", err)
	}
}
