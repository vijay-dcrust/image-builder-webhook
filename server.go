package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"

	"k8s.io/api/admission/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {

	http.HandleFunc("/", ExampleHandler)
	http.HandleFunc("/mutate", WebhookMutator)
	log.Println("** Service Started on Port 3000 **")

	// Use ListenAndServeTLS() instead of ListenAndServe() which accepts two extra parameters.
	// We need to specify both the certificate file and the key file (which we've named
	// https-server.crt and https-server.key).
	err := http.ListenAndServeTLS(":3000", "img-builder.crt", "img-builder.key", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	io.WriteString(w, `{"status":"ok"}`)
}

func WebhookMutator(w http.ResponseWriter, r *http.Request) {
	var body []byte
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	ctx := context.TODO()

	var isAllowed bool = false
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		log.Println("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	log.Println("Received request")

	if r.URL.Path != "/mutate" {
		log.Println("no mutate")
		http.Error(w, "no mutate", http.StatusBadRequest)
		return
	}

	arRequest := v1beta1.AdmissionReview{}
	if err := json.Unmarshal(body, &arRequest); err != nil {
		http.Error(w, "incorrect body", http.StatusBadRequest)
	}
	//raw := arRequest.Request.Object.Raw
	pod := v1.Pod{}
	if err := json.Unmarshal(body, &pod); err != nil {
		log.Println("error deserializing pod")
		return
	}
	if pod.Name == "kaniko" {
		isAllowed = true
		dupPod := CopyPod(&pod)
		log.Println(dupPod.Name)
		log.Println(dupPod.Spec.Containers[0].Image)
		_, err := clientset.CoreV1().Pods(pod.Namespace).Create(ctx, dupPod, metav1.CreateOptions{})
		if err != nil {
			log.Print("Panic. Pod didnt create successfully")
		}
		log.Print("Pod created successfully")
	}
	arResponse := v1beta1.AdmissionReview{
		Response: &v1beta1.AdmissionResponse{
			Allowed: isAllowed,
			Result: &metav1.Status{
				Message: "Keep calm and not add more crap in the cluster!",
			},
		},
	}
	resp, err := json.Marshal(arResponse)
	if err != nil {
		log.Printf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	log.Println("Ready to write reponse ...")
	if _, err := w.Write(resp); err != nil {
		log.Printf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}

}

func CopyPod(orig *v1.Pod) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kaniko-dup",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:         "kaniko-container",
					Image:        orig.Spec.Containers[0].Image,
					Args:         orig.Spec.Containers[0].Args,
					Env:          orig.Spec.Containers[0].Env,
					VolumeMounts: orig.Spec.Containers[0].VolumeMounts,
				},
			},
			RestartPolicy: orig.Spec.RestartPolicy,
			Volumes:       orig.Spec.Volumes,
		},
	}
}
