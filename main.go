package main

// Reference: https://dev.to/narasimha1997/create-kubernetes-jobs-in-golang-using-k8s-client-go-api-59ej

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type KanikoDispatcher struct {
	namespace string
	httpPort  string
	k8sClient *kubernetes.Clientset
}

type JobRequest struct {
	Name        string `json:"name"`
	Context     string `json:"context"`
	Destination string `json:"destination"`
	Secret      string `json:"secret"`
}

type JobQuery struct {
	Name string `json:"name"`
}

func connectToK8s() *kubernetes.Clientset {
	kubeconfig_file, exists := os.LookupEnv("KUBECONFIG")
	if !exists {
		log.Fatalln("Could not find KUBECONFIG environment variable.")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig_file)
	if err != nil {
		log.Panicln("failed to create K8s config", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Panicln("Failed to create K8s clientset", err)
	}
	return client

}

func (k *KanikoDispatcher) launchK8sJob(jobRequest *JobRequest, namespace string) (*batchv1.Job, error) {
	jobs := k.k8sClient.BatchV1().Jobs(namespace)

	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobRequest.Name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  jobRequest.Name,
							Image: "gcr.io/kaniko-project/executor:latest",
						},
					},
					RestartPolicy: v1.RestartPolicyNever,
				},
			},
		},
	}

	resp, err := jobs.Create(context.TODO(), jobSpec, metav1.CreateOptions{})
	return resp, err
}

func (k *KanikoDispatcher) web() {
	ginServer := gin.Default()

	ginServer.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "Ready for jobs (POST /kaniko)",
		})
	})

	ginServer.POST("/kaniko", func(c *gin.Context) {
		jobRequest := &JobRequest{}
		c.BindJSON(jobRequest)

		log.Println("Launching " + jobRequest.Name + "...")
		job, err := k.launchK8sJob(jobRequest, k.namespace)
		if err != nil {
			msg := "Failed to launch " + jobRequest.Name + ": " + err.Error()
			log.Println(msg)
			c.JSON(500, gin.H{"error": msg})
			return
		}
		msg := jobRequest.Name + " launched successfully as " + job.Name
		log.Println(msg)
		c.JSON(200, gin.H{"message": msg, "name": job.Name})
	})

	ginServer.GET("/kaniko", func(c *gin.Context) {
		jobQuery := &JobQuery{}
		c.BindJSON(jobQuery)
		// check the job status
		jobs := k.k8sClient.BatchV1().Jobs(k.namespace)
		job, err := jobs.Get(context.TODO(), jobQuery.Name, metav1.GetOptions{})
		if err != nil {
			msg := "Failed to get job status for" + jobQuery.Name + ": " + err.Error()
			log.Println(msg)
			c.JSON(500, gin.H{"error": msg})
			return
		}

		// check if the job is complete
		if job.Status.Succeeded == 1 {
			log.Println(jobQuery.Name + " completed successfully")
			c.JSON(200, gin.H{"message": jobQuery.Name + " completed successfully"})
			return
		}
		// check if the job is failed
		if job.Status.Failed == 1 {
			log.Println(jobQuery.Name + " failed")
			c.JSON(500, gin.H{"error": jobQuery.Name + " failed"})
			return
		}
		// check if the job is running
		if job.Status.Active == 1 {
			log.Println(jobQuery.Name + " is running")
			c.JSON(200, gin.H{"message": jobQuery.Name + " is running"})
			return
		}

	})

	ginServer.Run(":" + k.httpPort)
}

func main() {

	namespace := flag.String("namespace", "kaniko", "Default namespace to run Kaniko jobs in")
	httpPort := flag.String("http", "8080", "HTTP port to listen on")
	flag.Parse()

	engine := KanikoDispatcher{
		k8sClient: connectToK8s(),
		namespace: *namespace,
		httpPort:  *httpPort,
	}

	go engine.web()
	select {}
}