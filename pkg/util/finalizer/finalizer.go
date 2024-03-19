package finalizer

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func Remove(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Delete(finalizer)
	object.SetFinalizers(finalizers.List())
}

// Add adds a finalizer to an object
func Add(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Insert(finalizer)
	object.SetFinalizers(finalizers.List())
}

// WasDeleteRequested verifies if the resource was requested for deletion
func WasDeleteRequested(o v1.Object) bool {
	return o.GetDeletionTimestamp() != nil
}

// HasFinalizer verifies if a finalizer is placed on the resource
func HasFinalizer(o v1.Object, finalizerKey string) bool {
	return Contains(o.GetFinalizers(), finalizerKey)
}
