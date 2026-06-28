# Makefile for Self-Healing Platform Control Plane Validation & Thesis Benchmarking

.PHONY: all build test demo chaos rollback-demo canary-demo incident-demo load-tests clean

all: build test

build:
	go build ./...

test:
	go test -v ./...

demo:
	go run cmd/demo/main.go

chaos:
	@echo "=========================================================================="
	@echo "                     CHAOS ENGINEERING EXPERIMENTS                        "
	@echo "=========================================================================="
	@echo "Injecting real and simulated failure scenarios into the platform..."
	@echo ""
	go run cmd/demo/main.go

rollback-demo:
	@echo "=========================================================================="
	@echo "                     AUTOMATED HELM SDK ROLLBACK DEMO                    "
	@echo "=========================================================================="
	@echo "Simulating service crash to trigger automated Helm rollback..."
	@echo ""
	go run cmd/demo/main.go

canary-demo:
	@echo "=========================================================================="
	@echo "                 CANARY PROGRESSIVE DELIVERY ABORT DEMO                   "
	@echo "=========================================================================="
	@echo "Simulating canary deployment guard failure and burn rate SLO violation..."
	@echo ""
	go run cmd/demo/main.go

incident-demo:
	@echo "=========================================================================="
	@echo "                 INCIDENT LIFECYCLE & TIMELINE DEMO                       "
	@echo "=========================================================================="
	@echo "Simulating full active/historical incident transitions & REST responses..."
	@echo ""
	go run cmd/demo/main.go

load-tests:
	@echo "=========================================================================="
	@echo "                         k6 LOAD TESTING SCRIPTS                          "
	@echo "=========================================================================="
	@echo " k6 files are located at: ./load-testing/"
	@echo " -> To execute, make sure k6 is installed and run:"
	@echo "    k6 run ./load-testing/low_load.js"
	@echo "    k6 run ./load-testing/medium_load.js"
	@echo "    k6 run ./load-testing/high_load.js"
	@echo "    k6 run ./load-testing/stress.js"
	@echo "    k6 run ./load-testing/spike.js"
	@echo "    k6 run ./load-testing/soak.js"

clean:
	rm -rf docs/experiments/report-*.md docs/experiments/benchmark-comparison.md
