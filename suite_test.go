package pgxprof_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPgxprof(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pgxprof Suite")
}
