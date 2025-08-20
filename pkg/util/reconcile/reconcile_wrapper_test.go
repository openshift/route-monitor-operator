package reconcile_test

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/openshift/route-monitor-operator/pkg/util/reconcile"
)

var _ = Describe("ReconcileWrapper", func() {

	Describe("Result.Convert", func() {
		It("should convert Result to ctrl.Result correctly", func() {
			result := reconcile.Result{
				Requeue:      true,
				RequeueAfter: 5 * time.Second,
				Continue:     false,
			}

			ctrlResult := result.Convert()

			Expect(ctrlResult.Requeue).To(BeTrue())
			Expect(ctrlResult.RequeueAfter).To(Equal(5 * time.Second))
		})
	})

	Describe("Result.ReturnWith", func() {
		It("should return ctrl.Result and error", func() {
			result := reconcile.Result{
				Requeue:      true,
				RequeueAfter: 5 * time.Second,
			}
			testError := errors.New("test error")

			ctrlResult, err := result.ReturnWith(testError)

			Expect(ctrlResult.Requeue).To(BeTrue())
			Expect(ctrlResult.RequeueAfter).To(Equal(5 * time.Second))
			Expect(err).To(Equal(testError))
		})
	})

	Describe("Stop", func() {
		It("should return empty ctrl.Result and no error", func() {
			ctrlResult, err := reconcile.Stop()

			Expect(ctrlResult).To(Equal(ctrl.Result{}))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("RequeueWith", func() {
		It("should return empty ctrl.Result and the provided error", func() {
			testError := errors.New("test error")

			ctrlResult, err := reconcile.RequeueWith(testError)

			Expect(ctrlResult).To(Equal(ctrl.Result{}))
			Expect(err).To(Equal(testError))
		})
	})

	Describe("Requeue", func() {
		It("should return ctrl.Result with Requeue true and no error", func() {
			ctrlResult, err := reconcile.Requeue()

			Expect(ctrlResult.Requeue).To(BeTrue())
			Expect(ctrlResult.RequeueAfter).To(Equal(time.Duration(0)))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("RequeueAfter", func() {
		It("should return ctrl.Result with RequeueAfter set", func() {
			duration := 10 * time.Second

			ctrlResult := reconcile.RequeueAfter(duration)

			Expect(ctrlResult.RequeueAfter).To(Equal(duration))
			Expect(ctrlResult.Requeue).To(BeFalse())
		})
	})

	Describe("Result.RequeueOrStop", func() {
		It("should return true when Requeue is true", func() {
			result := reconcile.Result{Requeue: true, Continue: false}
			Expect(result.RequeueOrStop()).To(BeTrue())
		})

		It("should return true when Continue is false", func() {
			result := reconcile.Result{Requeue: false, Continue: false}
			Expect(result.RequeueOrStop()).To(BeTrue())
		})

		It("should return false when Requeue is false and Continue is true", func() {
			result := reconcile.Result{Requeue: false, Continue: true}
			Expect(result.RequeueOrStop()).To(BeFalse())
		})
	})

	Describe("Result.ShouldStop", func() {
		It("should return true when Continue is false", func() {
			result := reconcile.Result{Continue: false}
			Expect(result.ShouldStop()).To(BeTrue())
		})

		It("should return false when Continue is true", func() {
			result := reconcile.Result{Continue: true}
			Expect(result.ShouldStop()).To(BeFalse())
		})
	})

	Describe("RequeueReconcile", func() {
		It("should return Result with Requeue true and no error", func() {
			result, err := reconcile.RequeueReconcile()

			Expect(result.Requeue).To(BeTrue())
			Expect(result.Continue).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

