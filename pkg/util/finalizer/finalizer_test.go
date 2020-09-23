package finalizer_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"
)

var _ = Describe("Finalizer", func() {
	const (
		key = "fake-key"
	)
	var (
		list []string
	)

	Describe("Contains", func() {
		When("list is empty and key is defined", func() {
			// Arrange
			BeforeEach(func() {
				list = []string{}
			})
			It("should Return false for a key", func() {
				// Act
				res := finalizer.Contains(list, key)
				// Asset
				Expect(res).To(BeFalse())
			})
		})
		When("list is full and key is not there", func() {
			// Arrange
			BeforeEach(func() {
				list = []string{"second-fake-key"}
			})
			It("should Return false for a key isn't in the list", func() {
				// Act
				res := finalizer.Contains(list, key)
				// Asset
				Expect(res).To(BeFalse())
			})
		})
		When("list is full and key is there", func() {
			// Arrange
			BeforeEach(func() {
				list = []string{key}
			})
			It("should Return true for a key is actually there", func() {
				// Act
				res := finalizer.Contains(list, key)
				// Asset
				Expect(res).To(BeTrue())
			})
		})
	})
})
