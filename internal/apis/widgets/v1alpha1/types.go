// Package v1alpha1 contains the external (wire) API types for the
// widgets.example.com group, exposed via Kubernetes API aggregation.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupName is the API group served by this service.
const GroupName = "widgets.example.com"

// SchemeGroupVersion is the group/version used to register these types.
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

// SchemeBuilder collects functions that add types to a scheme.
var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Widget{},
		&WidgetList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Widget is the external representation of a widget resource.
type Widget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WidgetSpec   `json:"spec,omitempty"`
	Status WidgetStatus `json:"status,omitempty"`
}

type WidgetSpec struct {
	Size  string `json:"size"`
	Color string `json:"color,omitempty"`
}

type WidgetStatus struct {
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WidgetList is a list of Widgets.
type WidgetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Widget `json:"items"`
}

// DeepCopyObject implements runtime.Object.
func (in *Widget) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(Widget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is a manually-written deepcopy (normally produced by
// controller-gen/deepcopy-gen; hand-rolled here to avoid a codegen
// dependency for this template).
func (in *Widget) DeepCopyInto(out *Widget) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy creates a deep copy of Widget.
func (in *Widget) DeepCopy() *Widget {
	if in == nil {
		return nil
	}
	out := new(Widget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements runtime.Object.
func (in *WidgetList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(WidgetList)
	in.DeepCopyInto(out)
	return out
}

func (in *WidgetList) DeepCopyInto(out *WidgetList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		items := make([]Widget, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&items[i])
		}
		out.Items = items
	}
}

func (in *WidgetList) DeepCopy() *WidgetList {
	if in == nil {
		return nil
	}
	out := new(WidgetList)
	in.DeepCopyInto(out)
	return out
}
