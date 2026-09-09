package restadapter

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"

	widgetsv1alpha1 "github.com/alan-kelly-maersk/kubernetes-api-service/internal/apis/widgets/v1alpha1"
	"github.com/alan-kelly-maersk/kubernetes-api-service/internal/domain/widget"
)

// genericNamespace extracts the request namespace using the standard
// apiserver request context, keeping this adapter decoupled from any HTTP
// framework specifics.
func genericNamespace(ctx context.Context) (string, bool) {
	return genericapirequest.NamespaceFrom(ctx)
}

// toExternal maps a domain Widget to its Kubernetes wire representation.
func toExternal(w *widget.Widget) *widgetsv1alpha1.Widget {
	if w == nil {
		return nil
	}
	out := &widgetsv1alpha1.Widget{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         w.Namespace,
			Name:              w.Name,
			UID:               types.UID(w.UID),
			ResourceVersion:   fmt.Sprintf("%d", w.ResourceVersion),
			CreationTimestamp: metav1.NewTime(w.CreatedAt),
		},
		Spec: widgetsv1alpha1.WidgetSpec{
			Size:  w.Size,
			Color: w.Color,
		},
		Status: widgetsv1alpha1.WidgetStatus{
			Phase:   string(w.Phase),
			Message: w.Message,
		},
	}
	if w.DeletedAt != nil {
		dt := metav1.NewTime(*w.DeletedAt)
		out.DeletionTimestamp = &dt
	}
	return out
}

// toDomain maps a Kubernetes wire object back to the domain model.
func toDomain(namespace string, in *widgetsv1alpha1.Widget) widget.Widget {
	var rv uint64
	fmt.Sscanf(in.ResourceVersion, "%d", &rv)
	return widget.Widget{
		Namespace:       namespace,
		Name:            in.Name,
		UID:             string(in.UID),
		ResourceVersion: rv,
		Size:            in.Spec.Size,
		Color:           in.Spec.Color,
		Phase:           widget.Phase(in.Status.Phase),
		Message:         in.Status.Message,
	}
}

// toAPIError translates domain sentinel errors into the apimachinery
// error types the apiserver machinery expects, so clients get correct
// HTTP status codes (404, 409, 422, ...).
func toAPIError(gr schema.GroupResource, name string, err error) error {
	switch err {
	case widget.ErrNotFound:
		return apierrors.NewNotFound(gr, name)
	case widget.ErrConflict:
		return apierrors.NewConflict(gr, name, err)
	case widget.ErrAlreadyExists:
		return apierrors.NewAlreadyExists(gr, name)
	default:
		return apierrors.NewInternalError(err)
	}
}
