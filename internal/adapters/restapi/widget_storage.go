// Package restadapter is the inbound (driving) adapter that exposes the
// Widget domain use cases as a Kubernetes rest.Storage implementation,
// consumed by the aggregated API server instead of the generic
// etcd-backed registry.
package restadapter

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/registry/rest"

	widgetsv1alpha1 "github.com/alan-kelly-maersk/kubernetes-api-service/internal/apis/widgets/v1alpha1"
	"github.com/alan-kelly-maersk/kubernetes-api-service/internal/domain/widget"
)

// WidgetStorage implements the rest.Storage family of interfaces required
// to serve Widgets, translating between the Kubernetes wire types and the
// domain model by calling the widget.Service use cases.
type WidgetStorage struct {
	svc *widget.Service
	gvr schema.GroupVersionResource
}

// NewWidgetStorage builds the REST adapter for a given use-case Service.
func NewWidgetStorage(svc *widget.Service) *WidgetStorage {
	return &WidgetStorage{
		svc: svc,
		gvr: widgetsv1alpha1.SchemeGroupVersion.WithResource("widgets"),
	}
}

var (
	_ rest.Storage              = &WidgetStorage{}
	_ rest.Scoper               = &WidgetStorage{}
	_ rest.Getter               = &WidgetStorage{}
	_ rest.Lister               = &WidgetStorage{}
	_ rest.Creater              = &WidgetStorage{}
	_ rest.Updater              = &WidgetStorage{}
	_ rest.GracefulDeleter      = &WidgetStorage{}
	_ rest.Watcher              = &WidgetStorage{}
	_ rest.SingularNameProvider = &WidgetStorage{}
)

func (s *WidgetStorage) New() runtime.Object     { return &widgetsv1alpha1.Widget{} }
func (s *WidgetStorage) NewList() runtime.Object { return &widgetsv1alpha1.WidgetList{} }
func (s *WidgetStorage) NamespaceScoped() bool   { return true }
func (s *WidgetStorage) GetSingularName() string { return "widget" }

// Destroy is a no-op: this storage holds no per-request resources that
// need releasing (the Postgres pool is owned and closed by main()).
func (s *WidgetStorage) Destroy() {}

// ConvertToTable provides the default `kubectl get` table rendering.
func (s *WidgetStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return rest.NewDefaultTableConvertor(s.gvr.GroupResource()).ConvertToTable(ctx, object, tableOptions)
}

func (s *WidgetStorage) Get(ctx context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {
	ns, _ := genericNamespace(ctx)
	w, err := s.svc.Get(ctx, ns, name)
	if err != nil {
		return nil, toAPIError(s.gvr.GroupResource(), name, err)
	}
	return toExternal(w), nil
}

func (s *WidgetStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	ns, _ := genericNamespace(ctx)
	limit := 0
	cont := ""
	if options != nil {
		if options.Limit > 0 {
			limit = int(options.Limit)
		}
		cont = options.Continue
	}
	items, err := s.svc.List(ctx, widget.ListFilter{Namespace: ns, Limit: limit, Continue: cont})
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	list := &widgetsv1alpha1.WidgetList{}
	for i := range items {
		list.Items = append(list.Items, *toExternal(&items[i]))
	}
	return list, nil
}

func (s *WidgetStorage) Create(ctx context.Context, obj runtime.Object, validate rest.ValidateObjectFunc, _ *metav1.CreateOptions) (runtime.Object, error) {
	in, ok := obj.(*widgetsv1alpha1.Widget)
	if !ok {
		return nil, apierrors.NewBadRequest("object is not a Widget")
	}
	if validate != nil {
		if err := validate(ctx, obj); err != nil {
			return nil, err
		}
	}
	ns, _ := genericNamespace(ctx)
	domainWidget := toDomain(ns, in)
	created, err := s.svc.Create(ctx, domainWidget)
	if err != nil {
		return nil, toAPIError(s.gvr.GroupResource(), in.Name, err)
	}
	return toExternal(created), nil
}

func (s *WidgetStorage) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, _ *metav1.UpdateOptions) (runtime.Object, bool, error) {
	ns, _ := genericNamespace(ctx)
	oldDomain, err := s.svc.Get(ctx, ns, name)
	created := false
	if err != nil {
		if err != widget.ErrNotFound || !forceAllowCreate {
			return nil, false, toAPIError(s.gvr.GroupResource(), name, err)
		}
		created = true
		oldDomain = &widget.Widget{Namespace: ns, Name: name}
	}
	oldExternal := toExternal(oldDomain)

	newObj, err := objInfo.UpdatedObject(ctx, oldExternal)
	if err != nil {
		return nil, false, err
	}
	newExternal, ok := newObj.(*widgetsv1alpha1.Widget)
	if !ok {
		return nil, false, apierrors.NewBadRequest("object is not a Widget")
	}

	if created {
		if createValidation != nil {
			if err := createValidation(ctx, newObj); err != nil {
				return nil, false, err
			}
		}
		result, err := s.svc.Create(ctx, toDomain(ns, newExternal))
		if err != nil {
			return nil, false, toAPIError(s.gvr.GroupResource(), name, err)
		}
		return toExternal(result), true, nil
	}

	if updateValidation != nil {
		if err := updateValidation(ctx, newObj, oldExternal); err != nil {
			return nil, false, err
		}
	}
	result, err := s.svc.Update(ctx, toDomain(ns, newExternal))
	if err != nil {
		return nil, false, toAPIError(s.gvr.GroupResource(), name, err)
	}
	return toExternal(result), false, nil
}

func (s *WidgetStorage) Delete(ctx context.Context, name string, validate rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	ns, _ := genericNamespace(ctx)
	current, err := s.svc.Get(ctx, ns, name)
	if err != nil {
		return nil, false, toAPIError(s.gvr.GroupResource(), name, err)
	}
	if validate != nil {
		if err := validate(ctx, toExternal(current)); err != nil {
			return nil, false, err
		}
	}
	var expectedRV uint64
	if options != nil && options.Preconditions != nil && options.Preconditions.ResourceVersion != nil {
		fmt.Sscanf(*options.Preconditions.ResourceVersion, "%d", &expectedRV)
	}
	deleted, err := s.svc.Delete(ctx, ns, name, expectedRV)
	if err != nil {
		return nil, false, toAPIError(s.gvr.GroupResource(), name, err)
	}
	return toExternal(deleted), true, nil
}

func (s *WidgetStorage) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	ns, _ := genericNamespace(ctx)
	var rv uint64
	if options != nil && options.ResourceVersion != "" {
		fmt.Sscanf(options.ResourceVersion, "%d", &rv)
	}
	events, err := s.svc.Watch(ctx, ns, rv)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}
	return newEventWatcher(ctx, events), nil
}
