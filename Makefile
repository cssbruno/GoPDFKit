all : documentation

documentation :

GO_PACKAGES ?= ./...
VERSION ?= $(shell sed -n '1p' VERSION 2>/dev/null)
TOOLS_DIR ?= tools
TOOLS_BIN ?= $(CURDIR)/$(TOOLS_DIR)/bin
TOOLCHAIN ?= $(shell awk '/^go / { print "go" $$2 "+auto"; exit }' $(TOOLS_DIR)/go.mod)
GOLANGCI_LINT := $(TOOLS_BIN)/golangci-lint
NILAWAY := $(TOOLS_BIN)/nilaway
GOSEC := $(TOOLS_BIN)/gosec
GOVULNCHECK := $(TOOLS_BIN)/govulncheck
BENCHSTAT := $(TOOLS_BIN)/benchstat
COMPLIANCE_OUT ?= artifacts/compliance
GENERATION_CORE_BENCH ?= BenchmarkGeneration(BaselineNoCompliance.*Concurrent40|TextConcurrent40|LongTextConcurrent40|UTF8Text.*Concurrent40|TextCompressionLevelConcurrent40|Images.*Concurrent40|SVGConcurrent40|TemplatesConcurrent40|ImportedPDFPagesConcurrent40|ProtectionConcurrent40|AttachmentsConcurrent40|HTMLLargeTableCompiled|HTMLWideTableCompiled)$
BENCH ?= BenchmarkGenerationHTMLLargeTableCompiled$
BENCH_PACKAGE ?= ./document
BENCH_COUNT ?= 3
BENCH_OUT ?= artifacts/benchmarks.txt
PAPER_ENGINE_BENCH_OUT ?= artifacts/paper-engine-benchmarks.txt
PAPER_ENGINE_BENCH_COUNT ?= 10
PAPER_ENGINE_BENCHTIME ?= 250ms
PAPER_ENGINE_CALIBRATION_PROFILE ?= docs/performance/calibrations/apple-m2-go1.26.json
GENERATION_CORE_BENCH_OUT ?= artifacts/generation-core-benchmarks.txt
PROFILE_DIR ?= artifacts/profiles
PROFILE_BENCHTIME ?= 10s
ALLOC_PROFILE_BENCHTIME ?= 20x
TRACE_BENCHTIME ?= 1s
PAPER_ENGINE_PROFILE_CPU_SECONDS ?= 2
PAPER_ENGINE_PROFILE_ALLOC_ITERATIONS ?= 20
PAPER_STUDIO_LATENCY_REPORT ?= artifacts/paper-studio-wasm-latency.json

.PHONY: all documentation cov coverage-check test race vet fmt-check check modules tools tools-clean benchstat lint lin nilaway gosec gosev govulncheck quality release-version release-check release-notes release-tag release-push release build bench bench-ci bench-generation-core bench-generation-core-ci bench-generation-core-budget bench-paper-engine bench-paper-engine-ci bench-paper-engine-budget bench-paper-studio bench-paper-studio-wasm-latency bench-paper-studio-wasm-latency-budget test-paper-studio-js test-paper-studio-wasm characterize-paper-engine paper-studio-wasm paper-studio profile-paper-engine profile-paper-engine-check profile profile-cpu profile-alloc profile-block profile-mutex profile-trace compliance-fixtures compliance-validate compliance-baseline-check compliance-regenerate pdf-reader-smoke clean

cov : all
	go test $(GO_PACKAGES) -coverprofile=coverage && go tool cover -html=coverage -o=coverage.html

coverage-check :
	sh tools/check-coverage.sh

test :
	go test $(GO_PACKAGES)

race :
	go test -race $(GO_PACKAGES)

vet :
	go vet $(GO_PACKAGES)

fmt-check :
	test -z "$$(gofmt -s -l .)"

check : test vet fmt-check

modules :
	sh tools/test-go-modules.sh

tools : $(GOLANGCI_LINT) $(NILAWAY) $(GOSEC) $(GOVULNCHECK) $(BENCHSTAT)

$(TOOLS_BIN) :
	mkdir -p "$(TOOLS_BIN)"

$(GOLANGCI_LINT) : tools/go.mod tools/go.sum | $(TOOLS_BIN)
	cd $(TOOLS_DIR) && GOBIN="$(TOOLS_BIN)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint

$(NILAWAY) : tools/go.mod tools/go.sum | $(TOOLS_BIN)
	cd $(TOOLS_DIR) && GOBIN="$(TOOLS_BIN)" go install go.uber.org/nilaway/cmd/nilaway

$(GOSEC) : tools/go.mod tools/go.sum | $(TOOLS_BIN)
	cd $(TOOLS_DIR) && GOBIN="$(TOOLS_BIN)" go install github.com/securego/gosec/v2/cmd/gosec

$(GOVULNCHECK) : tools/go.mod tools/go.sum | $(TOOLS_BIN)
	cd $(TOOLS_DIR) && GOBIN="$(TOOLS_BIN)" go install golang.org/x/vuln/cmd/govulncheck

$(BENCHSTAT) : tools/go.mod tools/go.sum | $(TOOLS_BIN)
	cd $(TOOLS_DIR) && GOBIN="$(TOOLS_BIN)" go install golang.org/x/perf/cmd/benchstat

benchstat : $(BENCHSTAT)

tools-clean :
	rm -rf "$(TOOLS_BIN)"

lint : $(GOLANGCI_LINT)
	GOTOOLCHAIN="$(TOOLCHAIN)" "$(GOLANGCI_LINT)" run $(GO_PACKAGES)

lin : lint

nilaway : $(NILAWAY)
	GOTOOLCHAIN="$(TOOLCHAIN)" "$(NILAWAY)" -exclude-test-files $(GO_PACKAGES)

gosec : $(GOSEC)
	GOTOOLCHAIN="$(TOOLCHAIN)" "$(GOSEC)" $(GO_PACKAGES)

gosev : gosec

govulncheck : $(GOVULNCHECK)
	GOTOOLCHAIN="$(TOOLCHAIN)" "$(GOVULNCHECK)" $(GO_PACKAGES)

quality : check coverage-check lint nilaway gosec govulncheck

release-version :
	@test -n "$(VERSION)" || (echo "VERSION is required, for example VERSION=v0.1.0" && exit 1)
	@printf '%s\n' "$(VERSION)" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(\.(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?$$' || (echo "VERSION must look like vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-PRERELEASE, got $(VERSION)" && exit 1)
	@test "$$(sed -n '1p' VERSION)" = "$(VERSION)" || (echo "VERSION file does not match requested release $(VERSION)" && exit 1)
	@grep -q "^## $(VERSION)" CHANGELOG.md || (echo "CHANGELOG.md is missing section $(VERSION)" && exit 1)

release-check : release-version check modules race coverage-check lint nilaway gosec govulncheck build

release-notes : release-version
	@awk -v version="$(VERSION)" 'BEGIN { found = 0 } /^## / { if (found) exit; if ($$2 == version) found = 1 } found { print } END { if (!found) exit 1 }' CHANGELOG.md

release-tag : release-check
	@test -z "$$(git status --porcelain)" || (echo "working tree must be clean before tagging"; git status --short; exit 1)
	@test -z "$$(git tag --list "$(VERSION)")" || (echo "tag $(VERSION) already exists" && exit 1)
	git tag -a "$(VERSION)" -m "Release $(VERSION)"

release-push :
	git push origin "$(VERSION)"

release : release-tag

build : paper-studio-wasm
	go build -v $(GO_PACKAGES)

bench :
	go test $(GO_PACKAGES) -run '^$$' -bench . -benchmem

bench-ci :
	sh tools/run-benchmark.sh "$(BENCH_OUT)" go test $(GO_PACKAGES) -run '^$$' -bench . -benchmem -count="$(BENCH_COUNT)"

bench-generation-core :
	go test ./document -run '^$$' -bench '$(value GENERATION_CORE_BENCH)' -benchmem

bench-generation-core-ci :
	sh tools/run-benchmark.sh "$(GENERATION_CORE_BENCH_OUT)" go test ./document -run '^$$' -bench '$(value GENERATION_CORE_BENCH)' -benchmem -count="$(BENCH_COUNT)"

bench-generation-core-budget : bench-generation-core-ci
	sh tools/benchmark-budget-check.sh "$(GENERATION_CORE_BENCH_OUT)"

bench-paper-engine :
	go test ./document ./internal/layoutengine -run '^$$' -bench '^BenchmarkPaperEngine(Planner|Painter|ProductionDefault|EndToEnd|WarmCompiled|Concurrent|Table)' -benchmem

bench-paper-engine-ci :
	PAPER_ENGINE_BENCH_COUNT="$(PAPER_ENGINE_BENCH_COUNT)" PAPER_ENGINE_BENCHTIME="$(PAPER_ENGINE_BENCHTIME)" sh tools/run-paper-engine-benchmarks.sh "$(PAPER_ENGINE_BENCH_OUT)"

bench-paper-engine-budget : bench-paper-engine-ci
	PAPER_ENGINE_CALIBRATION_PROFILE="$(PAPER_ENGINE_CALIBRATION_PROFILE)" sh tools/check-paper-engine-benchmark-report.sh "$(PAPER_ENGINE_BENCH_OUT)"

bench-paper-studio :
	go test ./cmd/paper-studio -run '^$$' -bench BenchmarkPaperStudio -benchmem -benchtime=250ms -count=5

bench-paper-studio-wasm-latency : paper-studio-wasm
	PAPER_STUDIO_BENCH_SAMPLES="$${PAPER_STUDIO_BENCH_SAMPLES:-10}" PAPER_STUDIO_LATENCY_REPORT="$(PAPER_STUDIO_LATENCY_REPORT)" sh tools/benchmark-paper-studio-wasm.sh

bench-paper-studio-wasm-latency-budget : bench-paper-studio-wasm-latency
	node tools/check-paper-studio-latency-report.mjs "$(PAPER_STUDIO_LATENCY_REPORT)"

test-paper-studio-js :
	node --test cmd/paper-studio/js_test/*.cjs

PAPER_STUDIO_WASM := cmd/paper-studio/web/paper-studio.wasm
PAPER_STUDIO_WASM_EXEC := cmd/paper-studio/web/wasm_exec.js

paper-studio-wasm :
	GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o "$(PAPER_STUDIO_WASM)" ./cmd/paper-studio-wasm
	chmod u+w "$(PAPER_STUDIO_WASM_EXEC)" 2>/dev/null || true
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" "$(PAPER_STUDIO_WASM_EXEC)"
	chmod 0644 "$(PAPER_STUDIO_WASM_EXEC)"

test-paper-studio-wasm : paper-studio-wasm
	sh tools/test-paper-studio-wasm.sh

characterize-paper-engine :
	mkdir -p artifacts/characterization
	go run ./cmd/paper-characterize -builtin typed > artifacts/characterization/typed.json
	go run ./cmd/paper-characterize -builtin html > artifacts/characterization/html.json

PAPER_STUDIO_FILE ?= testdata/paper/studio-demo.paper
PAPER_STUDIO_ADDR ?= 127.0.0.1:7331

paper-studio : paper-studio-wasm
	go run ./cmd/paper-studio -addr "$(PAPER_STUDIO_ADDR)" "$(PAPER_STUDIO_FILE)"

profile-paper-engine :
	PAPER_ENGINE_PROFILE_CPU_SECONDS="$(PAPER_ENGINE_PROFILE_CPU_SECONDS)" PAPER_ENGINE_PROFILE_ALLOC_ITERATIONS="$(PAPER_ENGINE_PROFILE_ALLOC_ITERATIONS)" sh tools/run-paper-engine-profiles.sh "$(PROFILE_DIR)/paper-engine"

profile-paper-engine-check : profile-paper-engine
	sh tools/check-paper-engine-profile-report.sh "$(PROFILE_DIR)/paper-engine"

profile : profile-cpu profile-alloc profile-block profile-mutex profile-trace

profile-cpu :
	mkdir -p "$(PROFILE_DIR)"
	go test "$(BENCH_PACKAGE)" -run '^$$' -bench '$(value BENCH)' -benchtime="$(PROFILE_BENCHTIME)" -count=1 -o="$(abspath $(PROFILE_DIR))/" -outputdir="$(abspath $(PROFILE_DIR))" -cpuprofile=cpu.pprof

profile-alloc :
	mkdir -p "$(PROFILE_DIR)"
	go test "$(BENCH_PACKAGE)" -run '^$$' -bench '$(value BENCH)' -benchtime="$(ALLOC_PROFILE_BENCHTIME)" -count=1 -o="$(abspath $(PROFILE_DIR))/" -outputdir="$(abspath $(PROFILE_DIR))" -memprofile=alloc.pprof -memprofilerate=1

profile-block :
	mkdir -p "$(PROFILE_DIR)"
	go test "$(BENCH_PACKAGE)" -run '^$$' -bench '$(value BENCH)' -benchtime="$(PROFILE_BENCHTIME)" -count=1 -o="$(abspath $(PROFILE_DIR))/" -outputdir="$(abspath $(PROFILE_DIR))" -blockprofile=block.pprof -blockprofilerate=1

profile-mutex :
	mkdir -p "$(PROFILE_DIR)"
	go test "$(BENCH_PACKAGE)" -run '^$$' -bench '$(value BENCH)' -benchtime="$(PROFILE_BENCHTIME)" -count=1 -o="$(abspath $(PROFILE_DIR))/" -outputdir="$(abspath $(PROFILE_DIR))" -mutexprofile=mutex.pprof -mutexprofilefraction=1

profile-trace :
	mkdir -p "$(PROFILE_DIR)"
	go test "$(BENCH_PACKAGE)" -run '^$$' -bench '$(value BENCH)' -benchtime="$(TRACE_BENCHTIME)" -count=1 -o="$(abspath $(PROFILE_DIR))/" -outputdir="$(abspath $(PROFILE_DIR))" -trace=trace.out

compliance-fixtures :
	go run ./cmd/compliance-fixtures -out "$(COMPLIANCE_OUT)" $(if $(SRGB_ICC),-icc "$(SRGB_ICC)") -report "$(COMPLIANCE_OUT)/characterization.json"

compliance-validate :
	COMPLIANCE_OUT="$(COMPLIANCE_OUT)" SRGB_ICC="$(SRGB_ICC)" VERAPDF="$(VERAPDF)" PDFUA_CHECKER="$(PDFUA_CHECKER)" ARLINGTON_CHECKER="$(ARLINGTON_CHECKER)" ARLINGTON_URL="$(ARLINGTON_URL)" ARLINGTON_PROFILE="$(ARLINGTON_PROFILE)" ARLINGTON_REPORT_DIR="$(ARLINGTON_REPORT_DIR)" REQUIRE_COMPLIANCE_TOOLS="$(REQUIRE_COMPLIANCE_TOOLS)" sh tools/compliance-validate.sh

compliance-baseline-check :
	COMPLIANCE_OUT="$(COMPLIANCE_OUT)" COMPLIANCE_BASELINE_DIR="$(COMPLIANCE_BASELINE_DIR)" VERAPDF_DOCKER_IMAGE="$(VERAPDF_DOCKER_IMAGE)" sh tools/compliance-baseline-check.sh "$(COMPLIANCE_OUT)"

compliance-regenerate :
	COMPLIANCE_OUT="$(COMPLIANCE_OUT)" SRGB_ICC="$(SRGB_ICC)" VERAPDF="$(VERAPDF)" PDFUA_CHECKER="$(PDFUA_CHECKER)" ARLINGTON_CHECKER="$(ARLINGTON_CHECKER)" ARLINGTON_URL="$(ARLINGTON_URL)" ARLINGTON_PROFILE="$(ARLINGTON_PROFILE)" ARLINGTON_REPORT_DIR="$(ARLINGTON_REPORT_DIR)" REQUIRE_COMPLIANCE_TOOLS="$(REQUIRE_COMPLIANCE_TOOLS)" sh tools/compliance-regenerate.sh

pdf-reader-smoke :
	sh tools/pdf-reader-smoke.sh

clean :
	rm -f coverage.html coverage
	rm -rf artifacts
