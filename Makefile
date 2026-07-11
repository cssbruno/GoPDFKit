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
GOSEC_EXCLUDES ?= G115
COMPLIANCE_OUT ?= artifacts/compliance
GENERATION_CORE_BENCH ?= BenchmarkGeneration(BaselineNoCompliance.*Concurrent40|TextConcurrent40|LongTextConcurrent40|UTF8Text.*Concurrent40|TextCompressionLevelConcurrent40|Images.*Concurrent40|SVGConcurrent40|TemplatesConcurrent40|ImportedPDFPagesConcurrent40|ProtectionConcurrent40|AttachmentsConcurrent40|HTMLLargeTableCompiled|HTMLWideTableCompiled)$
BENCH ?= BenchmarkGenerationHTMLLargeTableCompiled$
BENCH_PACKAGE ?= ./document
BENCH_COUNT ?= 3
BENCH_OUT ?= artifacts/benchmarks.txt
GENERATION_CORE_BENCH_OUT ?= artifacts/generation-core-benchmarks.txt
PROFILE_DIR ?= artifacts/profiles
PROFILE_BENCHTIME ?= 10s
ALLOC_PROFILE_BENCHTIME ?= 20x
TRACE_BENCHTIME ?= 1s

.PHONY: all documentation cov coverage-check test race vet fmt-check check modules tools tools-clean benchstat lint lin nilaway gosec gosev govulncheck quality release-version release-check release-notes release-tag release-push release build bench bench-ci bench-generation-core bench-generation-core-ci bench-generation-core-budget profile profile-cpu profile-alloc profile-block profile-mutex profile-trace compliance-fixtures compliance-validate compliance-baseline-check compliance-regenerate clean

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
	GOTOOLCHAIN="$(TOOLCHAIN)" "$(NILAWAY)" $(GO_PACKAGES)

gosec : $(GOSEC)
	GOTOOLCHAIN="$(TOOLCHAIN)" "$(GOSEC)" -exclude="$(GOSEC_EXCLUDES)" $(GO_PACKAGES)

gosev : gosec

govulncheck : $(GOVULNCHECK)
	GOTOOLCHAIN="$(TOOLCHAIN)" "$(GOVULNCHECK)" $(GO_PACKAGES)

quality : check coverage-check lint gosec govulncheck

release-version :
	@test -n "$(VERSION)" || (echo "VERSION is required, for example VERSION=v0.1.0" && exit 1)
	@case "$(VERSION)" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo "VERSION must look like vMAJOR.MINOR.PATCH, got $(VERSION)" && exit 1 ;; esac
	@grep -q "^## $(VERSION)" CHANGELOG.md || (echo "CHANGELOG.md is missing section $(VERSION)" && exit 1)

release-check : release-version check modules race coverage-check lint gosec govulncheck build

release-notes : release-version
	@awk -v version="$(VERSION)" 'BEGIN { found = 0 } /^## / { if (found) exit; if ($$2 == version) found = 1 } found { print } END { if (!found) exit 1 }' CHANGELOG.md

release-tag : release-check
	@test -z "$$(git status --porcelain)" || (echo "working tree must be clean before tagging"; git status --short; exit 1)
	@test -z "$$(git tag --list "$(VERSION)")" || (echo "tag $(VERSION) already exists" && exit 1)
	git tag -a "$(VERSION)" -m "Release $(VERSION)"

release-push :
	git push origin "$(VERSION)"

release : release-tag

build :
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
	go run ./cmd/compliance-fixtures -out "$(COMPLIANCE_OUT)" $(if $(SRGB_ICC),-icc "$(SRGB_ICC)")

compliance-validate :
	COMPLIANCE_OUT="$(COMPLIANCE_OUT)" SRGB_ICC="$(SRGB_ICC)" VERAPDF="$(VERAPDF)" PDFUA_CHECKER="$(PDFUA_CHECKER)" ARLINGTON_CHECKER="$(ARLINGTON_CHECKER)" ARLINGTON_URL="$(ARLINGTON_URL)" ARLINGTON_PROFILE="$(ARLINGTON_PROFILE)" ARLINGTON_REPORT_DIR="$(ARLINGTON_REPORT_DIR)" REQUIRE_COMPLIANCE_TOOLS="$(REQUIRE_COMPLIANCE_TOOLS)" sh tools/compliance-validate.sh

compliance-baseline-check :
	COMPLIANCE_OUT="$(COMPLIANCE_OUT)" COMPLIANCE_BASELINE_DIR="$(COMPLIANCE_BASELINE_DIR)" VERAPDF_DOCKER_IMAGE="$(VERAPDF_DOCKER_IMAGE)" sh tools/compliance-baseline-check.sh "$(COMPLIANCE_OUT)"

compliance-regenerate :
	COMPLIANCE_OUT="$(COMPLIANCE_OUT)" SRGB_ICC="$(SRGB_ICC)" VERAPDF="$(VERAPDF)" PDFUA_CHECKER="$(PDFUA_CHECKER)" ARLINGTON_CHECKER="$(ARLINGTON_CHECKER)" ARLINGTON_URL="$(ARLINGTON_URL)" ARLINGTON_PROFILE="$(ARLINGTON_PROFILE)" ARLINGTON_REPORT_DIR="$(ARLINGTON_REPORT_DIR)" REQUIRE_COMPLIANCE_TOOLS="$(REQUIRE_COMPLIANCE_TOOLS)" sh tools/compliance-regenerate.sh

clean :
	rm -f coverage.html coverage
	rm -f assets/generated/pdf/*.pdf
