.PHONY: release-test

release-test:
	goreleaser release --snapshot --clean
