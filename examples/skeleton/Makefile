### Global Variables
WORK_DIR = $(dir $(abspath $(firstword $(MAKEFILE_LIST))))
TMP_DIR = $(WORK_DIR).tmp
PRIVATE_REPOS = \
	cto-github.cisco.com

# patterns
null  =
space = $(null) #
comma = ,
# example of pseudo version is v0.11.1-0.20230615155859-57d47298acc1
# it's made up of tagged version, timestamp and commit hash
pseudo_version_pattern = (v[0-9]+(\.[A-Za-z0-9]+)+)-(0.)?[0-9]+-[A-Za-z0-9]+

### Main
.PHONY: init-once init-ws init-cli init print

default: init-cli

-include Makefile-Build
-include Makefile-Generated

## Required Variables from command-line
CLI_MOD ?= cto-github.cisco.com/NFV-BU/go-lanai
CLI_TAG ?=
FORCE ?=
UPGRADE ?=
DEV ?=

## Required Variables by Local Targets
GO ?= go
GIT ?= git
CLI ?= lanai-cli

INIT_TMP_DIR = $(TMP_DIR)/init
CLI_GOMOD_FILE = $(INIT_TMP_DIR)/go.mod
CLI_MOD_PATH = $(if $(CLI_TAG),$(CLI_MOD)@$(CLI_TAG),$(CLI_MOD))

## Local Targets

# init-once:
#	Used to setup local dev environment, and should be used only once per environment.
# 	This target assumes local environment has proper access to $PRIVATE_REPOS
init-once:
	$(GO) env -w GOPRIVATE="$(subst $(space),$(comma),$(strip $(value PRIVATE_REPOS)))"
	$(foreach repo,$(PRIVATE_REPOS),\
		$(GIT) config --global url."ssh://git@$(repo)/".insteadOf "https://$(repo)/";\
	)

# init-ws:
# 	create tmp folders
init-ws:
	@mkdir -p $(INIT_TMP_DIR)
	@[ -e "go.mod" ] && $(GO) mod edit -dropreplace $(CLI_MOD) -dropreplace $(CLI_MOD)@* -print > "$(CLI_GOMOD_FILE)"
	@[ -e "go.sum" ] && cp go.sum $(INIT_TMP_DIR)/go.sum
	@cd $(INIT_TMP_DIR); $(GO) mod download $(CLI_MOD_PATH);

# init-cli:
#	Used to bootstrap any targets other than init-once
# 	Required Vars:
#		- CLI_TAG branch/tag to use to install "lanai-cli", typically same branch/tag of cto-github.cisco.com/NFV-BU/go-lanai
#		  e.g. CLI_TAG=develop
init-cli: CLI_PKG = $(CLI_MOD)/cmd/lanai-cli
ifneq ($(filter true True TRUE,$(DEV)),)
# For CLI Dev, or Service Dev who have go-lanai checkout out and have proper "replace" directive in go.mod
init-cli: CLI_VERSION = service-$(shell $(GIT) rev-parse HEAD)
init-cli: CLI_PKG_PATH = $(if $(CLI_TAG),$(CLI_PKG)@$(CLI_TAG),$(CLI_PKG))
init-cli: print
	@echo Installing $(CLI_PKG_PATH) $(CLI_VERSION) ...
	@$(GO) install -ldflags="-X main.BuildVersion=$(CLI_VERSION)" $(CLI_PKG_PATH)
else
# For CI/CD and Service Dev
init-cli: CLI_VERSION = $(shell $(GO) list -modfile "$(CLI_GOMOD_FILE)" -m -f {{.Version}} $(CLI_MOD_PATH))
# If the version is a pseudo version, we want to use the commit hash (the last 12 characters) for the go install command
init-cli: CLI_PKG_PATH = $(if [["$(CLI_VERSION)" =~ $(pseudo_version_pattern)]],$(CLI_PKG)@$(shell printf $(CLI_VERSION) | tail -c 12),$(CLI_PKG)@$(CLI_VERSION))
init-cli: init-ws print
	@echo Installing $(CLI_PKG_PATH) $(CLI_VERSION) ...
	@cd $(INIT_TMP_DIR); $(GO) install -ldflags="-X main.BuildVersion=$(CLI_VERSION)" $(CLI_PKG_PATH)
endif

# init:
#	Used to bootstrap any targets other than init-once and init-cli
# 	Optional Vars:
#		- FORCE add --force flag to "lanai-cli init" when "true". Would overwrite Makefile-Build, Dockerfile, etc.
init: init-cli
	$(CLI) init -o ./ $(if $(filter true True TRUE,$(FORCE)),--force) $(if $(filter true True TRUE,$(UPGRADE)),--upgrade)

print:
	@echo "CLI_VERSION:  $(CLI_VERSION)"
	@echo "CLI_PKG_PATH: $(CLI_PKG_PATH)"
	@echo "CLI_TAG:      $(CLI_TAG)"
