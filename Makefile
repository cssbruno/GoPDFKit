all : documentation

documentation : doc/index.html doc.go README.md 

GO_PACKAGES ?= ./...
VERSION ?= $(shell sed -n '1p' VERSION 2>/dev/null)
TOOLS_DIR ?= tools
TOOLS_BIN ?= $(CURDIR)/$(TOOLS_DIR)/bin
TOOLCHAIN ?= $(shell awk '/^go / { print "go" $$2 "+auto"; exit }' $(TOOLS_DIR)/go.mod)
GOLANGCI_LINT := $(TOOLS_BIN)/golangci-lint
NILAWAY := $(TOOLS_BIN)/nilaway
GOSEC := $(TOOLS_BIN)/gosec
GOVULNCHECK := $(TOOLS_BIN)/govulncheck
GOSEC_EXCLUDES ?= G115,G304,G401,G405,G501,G503,G505,G703
COMPLIANCE_OUT ?= artifacts/compliance
GOPDFSUIT_BENCH_DIR ?= benchmarks/gopdfsuit
GENERATION_CORE_BENCH ?= BenchmarkGeneration(BaselineNoCompliance.*|Text(Concurrent40)?|LongText(Concurrent40)?|UTF8Text.*|TextCompressionLevel.*|Images.*|SVG(Concurrent40)?|Templates(Concurrent40)?|ImportedPDFPages(Concurrent40)?|Protection(Concurrent40)?|Attachments(Concurrent40)?)$

.PHONY: all documentation cov test vet fmt-check check tools tools-clean lint lin nilaway gosec gosev govulncheck quality release-version release-check release-notes release-tag release-push release build bench bench-ci bench-generation-core bench-generation-core-ci bench-gopdfsuit bench-gopdfsuit-ci compliance-fixtures compliance-validate compliance-baseline-check compliance-regenerate clean

cov : all
	go test $(GO_PACKAGES) -coverprofile=coverage && go tool cover -html=coverage -o=coverage.html

test :
	go test $(GO_PACKAGES)

vet :
	go vet $(GO_PACKAGES)

fmt-check :
	test -z "$$(gofmt -s -l .)"

check : test vet fmt-check

tools : $(GOLANGCI_LINT) $(NILAWAY) $(GOSEC) $(GOVULNCHECK)

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

quality : check lin nilaway gosev govulncheck

release-version :
	@test -n "$(VERSION)" || (echo "VERSION is required, for example VERSION=v0.1.0" && exit 1)
	@case "$(VERSION)" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo "VERSION must look like vMAJOR.MINOR.PATCH, got $(VERSION)" && exit 1 ;; esac
	@grep -q "^## $(VERSION)" CHANGELOG.md || (echo "CHANGELOG.md is missing section $(VERSION)" && exit 1)

release-check : release-version check govulncheck build

release-notes : release-version
	@awk -v version="$(VERSION)" 'BEGIN { found = 0 } /^## / { if (found) exit; if ($$2 == version) found = 1 } found { print } END { if (!found) exit 1 }' CHANGELOG.md

release-tag : release-check
	@test -z "$$(git status --porcelain)" || (echo "working tree must be clean before tagging"; git status --short; exit 1)
	@test -z "$$(git tag --list "$(VERSION)")" || (echo "tag $(VERSION) already exists" && exit 1)
	git tag -a "$(VERSION)" -m "Release $(VERSION)"

release-push :
	git push origin "$(VERSION)"

release : release-tag

README.md : doc/document.md
	pandoc --read=markdown --write=gfm < $< > $@

doc/index.html : doc/document.md doc/html.txt
	pandoc --read=markdown --write=html --template=doc/html.txt \
		--metadata pagetitle="GoPDFKit Document Generator" < $< > $@

doc.go : doc/document.md doc/go.awk
	pandoc --read=markdown --write=plain $< | awk --assign=package_name=gopdfkit --file=doc/go.awk > $@
	gofmt -s -w $@

build :
	go build -v $(GO_PACKAGES)

bench :
	go test $(GO_PACKAGES) -run '^$$' -bench . -benchmem

bench-ci :
	mkdir -p artifacts
	go test $(GO_PACKAGES) -run '^$$' -bench . -benchmem -count=3 | tee artifacts/benchmarks.txt

bench-generation-core :
	go test ./document -run '^$$' -bench '$(GENERATION_CORE_BENCH)' -benchmem

bench-generation-core-ci :
	mkdir -p artifacts
	go test ./document -run '^$$' -bench '$(GENERATION_CORE_BENCH)' -benchmem -count=3 | tee artifacts/generation-core-benchmarks.txt

bench-gopdfsuit :
	cd $(GOPDFSUIT_BENCH_DIR) && go test -run '^TestComparableOutputsArePDF$$' -bench 'Benchmark(GoPDFKit|GoPDFLib)' -benchmem

bench-gopdfsuit-ci :
	mkdir -p artifacts
	cd $(GOPDFSUIT_BENCH_DIR) && go test -run '^TestComparableOutputsArePDF$$' -bench 'Benchmark(GoPDFKit|GoPDFLib)' -benchmem -count=3 | tee ../../artifacts/gopdfsuit-benchmarks.txt

compliance-fixtures :
	go run ./cmd/compliance-fixtures -out "$(COMPLIANCE_OUT)" $(if $(SRGB_ICC),-icc "$(SRGB_ICC)")

compliance-validate :
	COMPLIANCE_OUT="$(COMPLIANCE_OUT)" SRGB_ICC="$(SRGB_ICC)" VERAPDF="$(VERAPDF)" PDFUA_CHECKER="$(PDFUA_CHECKER)" ARLINGTON_CHECKER="$(ARLINGTON_CHECKER)" ARLINGTON_URL="$(ARLINGTON_URL)" ARLINGTON_PROFILE="$(ARLINGTON_PROFILE)" ARLINGTON_REPORT_DIR="$(ARLINGTON_REPORT_DIR)" REQUIRE_COMPLIANCE_TOOLS="$(REQUIRE_COMPLIANCE_TOOLS)" sh tools/compliance-validate.sh

compliance-baseline-check :
	COMPLIANCE_OUT="$(COMPLIANCE_OUT)" COMPLIANCE_BASELINE_DIR="$(COMPLIANCE_BASELINE_DIR)" VERAPDF_DOCKER_IMAGE="$(VERAPDF_DOCKER_IMAGE)" sh tools/compliance-baseline-check.sh "$(COMPLIANCE_OUT)"

compliance-regenerate :
	COMPLIANCE_OUT="$(COMPLIANCE_OUT)" SRGB_ICC="$(SRGB_ICC)" VERAPDF="$(VERAPDF)" PDFUA_CHECKER="$(PDFUA_CHECKER)" ARLINGTON_CHECKER="$(ARLINGTON_CHECKER)" ARLINGTON_URL="$(ARLINGTON_URL)" ARLINGTON_PROFILE="$(ARLINGTON_PROFILE)" ARLINGTON_REPORT_DIR="$(ARLINGTON_REPORT_DIR)" REQUIRE_COMPLIANCE_TOOLS="$(REQUIRE_COMPLIANCE_TOOLS)" sh tools/compliance-regenerate.sh

clean :
	rm -f coverage.html coverage doc/index.html
	rm -f assets/generated/pdf/*.pdf
