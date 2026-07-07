// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// 模拟上游 API 服务（仅用于演示）
// 运行: go run mock_upstream.go

var (
	mockErrorRatio = 0.02
	mockMu         sync.RWMutex
)

var scenarios = map[string]float64{
	"normal":   0.02,
	"degraded": 0.08,
	"partial":  0.25,
	"outage":   0.6,
}

func main() {
	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/open/v1/upstream-status", func(w http.ResponseWriter, r *http.Request) {
		timeRange := 60
		if tr, err := strconv.Atoi(r.URL.Query().Get("time_range")); err == nil {
			timeRange = tr
		}

		mockMu.RLock()
		base := mockErrorRatio
		mockMu.RUnlock()

		// 添加随机波动
		fluctuation := (rand.Float64() - 0.5) * 0.02
		errorRatio := base + fluctuation
		if errorRatio < 0 {
			errorRatio = 0
		}
		if errorRatio > 1 {
			errorRatio = 1
		}

		now := time.Now().UTC()
		startTime := now.Add(-time.Duration(timeRange) * time.Minute)

		log.Printf("[Mock API] time_range=%dmin, error_ratio=%.2f%%", timeRange, errorRatio*100)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"message": "success",
			"data": map[string]interface{}{
				"time_range": map[string]string{
					"start_time": startTime.Format(time.RFC3339),
					"end_time":   now.Format(time.RFC3339),
				},
				"error_ratio": errorRatio,
			},
		})
	})

	// 控制接口：手动设置错误率
	http.HandleFunc("/mock/set-error", func(w http.ResponseWriter, r *http.Request) {
		ratio, err := strconv.ParseFloat(r.URL.Query().Get("ratio"), 64)
		if err != nil || ratio < 0 || ratio > 1 {
			http.Error(w, "Invalid ratio (0-1)", 400)
			return
		}

		mockMu.Lock()
		mockErrorRatio = ratio
		mockMu.Unlock()

		log.Printf("[Mock] Error ratio set to %.2f%%", ratio*100)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "error_ratio": ratio})
	})

	// 控制接口：使用预设场景
	http.HandleFunc("/mock/scenario/", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[len("/mock/scenario/"):]
		ratio, ok := scenarios[name]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":     "Unknown scenario",
				"available": []string{"normal", "degraded", "partial", "outage"},
			})
			return
		}

		mockMu.Lock()
		mockErrorRatio = ratio
		mockMu.Unlock()

		log.Printf("[Mock] Scenario \"%s\" activated, error_ratio=%.2f%%", name, ratio*100)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "scenario": name, "error_ratio": ratio})
	})

	fmt.Println(`
╔════════════════════════════════════════════════════════════╗
║           模拟上游 API 服务已启动                            ║
╠════════════════════════════════════════════════════════════╣
║  API 地址: http://localhost:8080/open/v1/upstream-status   ║
╠════════════════════════════════════════════════════════════╣
║  控制接口:                                                  ║
║  - /mock/set-error?ratio=0.1    设置错误率为 10%           ║
║  - /mock/scenario/normal        正常 (2%)                  ║
║  - /mock/scenario/degraded      性能下降 (8%)              ║
║  - /mock/scenario/partial       部分中断 (25%)             ║
║  - /mock/scenario/outage        严重故障 (60%)             ║
╚════════════════════════════════════════════════════════════╝`)

	log.Fatal(http.ListenAndServe(":8080", nil))
}
