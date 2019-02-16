package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang/glog"
	v1beta1 "k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	v1 "k8s.io/kubernetes/pkg/apis/core/v1"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()

	// (https://github.com/kubernetes/kubernetes/issues/57982)
	defaulter = runtime.ObjectDefaulter(runtimeScheme)
)

var ignoredNamespaces = []string{
	metav1.NamespaceSystem,
	metav1.NamespacePublic,
}

const (
	admissionWebhookAnnotationInjectKey      = "injector.doppler.com/inject"
	admissionWebhookAnnotationPipelineKey    = "injector.doppler.com/pipeline"
	admissionWebhookAnnotationEnvironmentKey = "injector.doppler.com/environment"
)

type WebhookServer struct {
	server      *http.Server
	client      *http.Client
	apiKey      string
	pipeline    string
	environment string
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func init() {
	_ = corev1.AddToScheme(runtimeScheme)
	_ = admissionregistrationv1beta1.AddToScheme(runtimeScheme)
	// defaulting with webhooks:
	// https://github.com/kubernetes/kubernetes/issues/57982
	_ = v1.AddToScheme(runtimeScheme)
}

// Check whether the target resoured need to be mutated
func mutationRequired(ignoredList []string, metadata *metav1.ObjectMeta) bool {
	// skip special kubernetes system namespaces
	for _, namespace := range ignoredList {
		if metadata.Namespace == namespace {
			glog.Infof("Skip mutation for %v for it' in special namespace:%v", metadata.Name, metadata.Namespace)
			return false
		}
	}

	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	// determine whether to perform mutation based on annotation for the target resource
	var required bool

	switch strings.ToLower(annotations[admissionWebhookAnnotationInjectKey]) {
	default:
		required = false
	case "y", "yes", "true", "on":
		required = true
	}

	glog.Infof("Mutation policy for %v/%v: required:%v", metadata.Namespace, metadata.Name, required)
	return required
}

func addEnv(target []corev1.Container, added []corev1.EnvVar, basePath string) (patch []patchOperation) {
	for index, container := range target {
		path := basePath + "/" + strconv.Itoa(index) + "/env"

		for _, envvar := range added {

			// Set container name if is from type resourceFieldRef
			if envvar.ValueFrom != nil && envvar.ValueFrom.ResourceFieldRef != nil {
				envvar.ValueFrom.ResourceFieldRef.ContainerName = container.Name
			}

			container.Env = append(container.Env, envvar)
		}

		patch = append(patch, patchOperation{
			Op:    "add",
			Path:  path,
			Value: container.Env,
		})

	}

	return patch
}

// create mutation patch for resources
func createPatch(pod *corev1.Pod, env []corev1.EnvVar) ([]byte, error) {
	var patch []patchOperation
	patch = append(patch, addEnv(pod.Spec.Containers, env, "/spec/containers")...)
	return json.Marshal(patch)
}

// main mutation process
func (whsvr *WebhookServer) mutate(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	req := ar.Request
	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		glog.Errorf("Could not unmarshal raw object: %v", err)
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v (%v) UID=%v patchOperation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, pod.Name, req.UID, req.Operation, req.UserInfo)

	// determine whether to perform mutation
	if !mutationRequired(ignoredNamespaces, &pod.ObjectMeta) {
		glog.Infof("Skipping mutation for %s/%s due to policy check", req.Namespace, pod.Name)
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	pipeline, environment := whsvr.getPipelineAndEnvironment(&pod.ObjectMeta)
	env, err := whsvr.getEnvFromDoppler(pipeline, environment)
	if err != nil {
		glog.Errorf("Failed to get values from doppler: %v", err)
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Infof("Retrieved values from doppler for %s/%s, using pipeline=%s, environment=%s", req.Namespace, pod.Name, pipeline, environment)

	patchBytes, err := createPatch(&pod, env)
	if err != nil {
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Info("AdmissionResponse successful")
	return &v1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

// Serve method for webhook server
func (whsvr *WebhookServer) serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		glog.Error("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		glog.Errorf("Can't decode body: %v", err)
		admissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		admissionResponse = whsvr.mutate(&ar)
	}

	admissionReview := v1beta1.AdmissionReview{}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if ar.Request != nil {
			admissionReview.Response.UID = ar.Request.UID
		}
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		glog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}

	if _, err := w.Write(resp); err != nil {
		glog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func (whsvr *WebhookServer) getPipelineAndEnvironment(metadata *metav1.ObjectMeta) (string, string) {
	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	pipeline := whsvr.pipeline
	environment := whsvr.environment

	if p, ok := annotations[admissionWebhookAnnotationPipelineKey]; ok {
		pipeline = p
	}

	if e, ok := annotations[admissionWebhookAnnotationEnvironmentKey]; ok {
		environment = e
	}

	return pipeline, environment
}
