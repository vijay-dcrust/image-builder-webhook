package admissioncontrol

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"k8s.io/api/admission/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var (
	armBuilderImage = "gcr.io/kaniko-project/executor:arm64-v1.3.0"
	x86BuilderImage = "gcr.io/kaniko-project/executor:v1.3.0"
	//armBuilderImage = "vijaydcrust/kaniko-latest"
	//x86BuilderImage = "vijaydcrust/kaniko-latest"
	armNodeSelector = map[string]string{
		"beta.kubernetes.io/arch": "arm64",
	}
	x86NodeSelector = map[string]string{
		"beta.kubernetes.io/arch": "amd64",
	}
	armTolerations = v1.Toleration{
		Effect: "NoSchedule",
		Key:    "node.kubernets.io/arch",
		Value:  "arm64",
	}
	x86Tolerations = v1.Toleration{}
)

// WebhookMutator  function to handle the admission review
func WebhookMutator(w http.ResponseWriter, r *http.Request) {
	var body []byte
	mode := os.Getenv("MODE")
	config := GetKubeConfig(mode)
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	var msg string
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

	//arRequest := v1beta1.AdmissionReview{}
	arRequest := v1beta1.AdmissionReview{}

	if err := json.Unmarshal(body, &arRequest); err != nil {
		http.Error(w, "incorrect body", http.StatusBadRequest)
	}
	raw := arRequest.Request.Object.Raw
	pod := v1.Pod{}
	if err := json.Unmarshal(raw, &pod); err != nil {
		log.Println("error deserializing pod")
		return
	}
	if pod.Labels["cross-platform-build"] != "enabled" {
		msg = "No action required for pod" + pod.Name
	} else {
		dupPod := CopyPod(pod)
		if dupPod == nil {
			msg = "No supported orig image found. " + pod.Name
		} else {
			log.Println(pod.Spec.Containers[0].Args[0])
			log.Println(pod.Spec.Containers[0].Args[1])
			log.Println(dupPod.Spec.Tolerations)
			_, err := clientset.CoreV1().Pods(pod.Namespace).Create(context.TODO(), dupPod, metav1.CreateOptions{})
			if err != nil {
				log.Println(err)
				msg = "Error. Cross platform Pod didnt create successfully for pod " + pod.Name
			} else {
				msg = "Cross platform Pod created successfully for pod " + pod.Name
			}
			log.Println(msg)
		}
	}
	arResponse := v1beta1.AdmissionReview{
		Response: &v1beta1.AdmissionResponse{
			Allowed: true,
			Result: &metav1.Status{
				Message: msg,
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

// CopyPod builds the cross platform Pod from existing Pod
func CopyPod(orig v1.Pod) *v1.Pod {
	var dupPodImage string
	var dupArgs []string
	var dupNodeSelector map[string]string
	dupTolerations := v1.Toleration{}
	if orig.Spec.Containers[0].Image == x86BuilderImage {
		dupPodImage = armBuilderImage
		dupArgs = UpdateDestinationArgs(orig.Spec.Containers[0].Args, "-arm-cross-platform-generated")
		dupNodeSelector = armNodeSelector
		dupTolerations = armTolerations
	} else if orig.Spec.Containers[0].Image == armBuilderImage {
		dupPodImage = x86BuilderImage
		dupArgs = UpdateDestinationArgs(orig.Spec.Containers[0].Args, "-x86-cross-platform-generated")
		dupNodeSelector = x86NodeSelector
		dupTolerations = x86Tolerations
	} else {
		log.Println("No supported image found ! image- " + orig.Spec.Containers[0].Image)
		return nil
	}
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dup-" + orig.Name + fmt.Sprint(rand.Intn(10000)),
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:         orig.Spec.Containers[0].Name,
					Image:        dupPodImage,
					Args:         dupArgs,
					Env:          orig.Spec.Containers[0].Env,
					VolumeMounts: orig.Spec.Containers[0].VolumeMounts,
				},
			},
			RestartPolicy: orig.Spec.RestartPolicy,
			NodeSelector:  dupNodeSelector,
			Volumes:       orig.Spec.Volumes,
			Tolerations:   []v1.Toleration{dupTolerations},
		},
	}
}

//UpdateDestinationArgs Update Destination image arguments for new architeture
func UpdateDestinationArgs(orig []string, imgTag string) []string {
	dupArgs := orig
	for i, s := range orig {
		match, _ := regexp.MatchString(`^--destination=`, s)
		if match == true {
			dupArgs[i] = orig[i] + imgTag
		}
		dupArgs[i] = orig[i]
	}
	return dupArgs
}

//GetKubeConfig Get the kubernetes config
func GetKubeConfig(mode string) *rest.Config {
	type config *rest.Config
	//Outside k8s Cluster authentication
	if mode == "DEV" {
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
		return config

	} else {
		//creates the in-cluster config
		config, err := rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
		return config
	}

}
