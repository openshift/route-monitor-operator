package finalizer

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Patch(object metav1.Object, prevFinalizer, finalizer string) {
	finalizers := object.GetFinalizers()
	hasPrev := Contains(finalizers, prevFinalizer)
	hasCurrent := Contains(finalizers, finalizer)
	switch {
	case hasPrev && !hasCurrent:
		Remove(object, prevFinalizer)
		Add(object, finalizer)
	case hasPrev && hasCurrent:
		Remove(object, prevFinalizer)
	}
}
