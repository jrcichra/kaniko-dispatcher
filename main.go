package main

// Reference: https://dev.to/narasimha1997/create-kubernetes-jobs-in-golang-using-k8s-client-go-api-59ej

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
)

type KanikoDispatcher struct {
	namespace string
	httpPort  string
	k8sClient *kubernetes.Clientset
}

type JobRequest struct {
	Name        string            `json:"name"`
	Context     string            `json:"context"`
	Destination string            `json:"destination"`
	Secret      string            `json:"secret"`
	Arch        string            `json:"arch,omitempty"`
	BuildArgs   map[string]string `json:"build_args,omitempty"`
}

type JobQuery struct {
	Name string `json:"name"`
}

func connectToK8s() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalln(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalln(err)
	}
	return clientset
}

func (k *KanikoDispatcher) cleanup() {
	t := time.NewTicker(time.Minute * 5)
	for {
		// wait for the ticker
		<-t.C

		log.Println("Cleaning up old jobs...")

		jobs := k.k8sClient.BatchV1().Jobs(k.namespace)
		list, err := jobs.List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			log.Println("Failed to list jobs", err)
			return
		}
		for _, job := range list.Items {
			// delete all jobs older than a day
			if job.CreationTimestamp.Add(24 * time.Hour).Before(time.Now()) {
				log.Println("Deleting job", job.Name)
				bg := metav1.DeletePropagationBackground
				err := jobs.Delete(context.TODO(), job.Name, metav1.DeleteOptions{
					PropagationPolicy: &bg,
				})
				if err != nil {
					log.Println("Failed to delete job", job.Name, err)
				}
			}
		}
	}
}

func (k *KanikoDispatcher) launchK8sJob(jobRequest *JobRequest, namespace string) (*batchv1.Job, error) {

	jobs := k.k8sClient.BatchV1().Jobs(namespace)

	// determine if nodeSelector needs to pick a specific architecture
	var nodeSelector map[string]string
	if jobRequest.Arch != "" {
		nodeSelector = map[string]string{"kubernetes.io/arch": jobRequest.Arch}
	}

	kanikoArgs := []string{
		"--context=" + jobRequest.Context,
		"--destination=" + jobRequest.Destination,
		"--cache=true",
	}
	for k, v := range jobRequest.BuildArgs {
		kanikoArgs = append(kanikoArgs, "--build-arg="+k+"="+v)
	}

	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobRequest.Name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: pointer.Int32(2),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					NodeSelector: nodeSelector,
					Containers: []v1.Container{
						{
							Name:            jobRequest.Name,
							Image:           "gcr.io/kaniko-project/executor:latest",
							ImagePullPolicy: v1.PullAlways,
							Args:            kanikoArgs,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kaniko-secret",
									MountPath: "/kaniko/.docker",
									ReadOnly:  true,
								},
							},
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("100m"),
									v1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Limits: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("800m"),
									v1.ResourceMemory: resource.MustParse("4096Mi"),
								},
							},
						},
					},
					RestartPolicy: v1.RestartPolicyNever,
					Volumes: []v1.Volume{
						{
							Name: "kaniko-secret",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName: jobRequest.Secret,
									Items: []v1.KeyToPath{
										{
											Key:  ".dockerconfigjson",
											Path: "config.json",
										},
									},
								},
							},
						},
					},
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
		jobQuery.Name = c.Query("name")
		// check the job status
		jobs := k.k8sClient.BatchV1().Jobs(k.namespace)
		job, err := jobs.Get(context.TODO(), jobQuery.Name, metav1.GetOptions{})
		if err != nil {
			msg := "Failed to get job status for '" + jobQuery.Name + "': " + err.Error()
			log.Println(msg)
			c.JSON(500, gin.H{"error": msg})
			return
		}

		// check if the job is complete
		if job.Status.Succeeded == 1 {
			log.Println(jobQuery.Name + " completed successfully")
			c.JSON(200, gin.H{"message": jobQuery.Name + " completed successfully", "done": true, "pass": true})
			return
		}
		// check if the job is failed
		if job.Status.Failed == 1 {
			log.Println(jobQuery.Name + " failed")
			c.JSON(500, gin.H{"message": jobQuery.Name + " failed", "done": true, "pass": false})
			return
		}
		// check if the job is running
		if job.Status.Active == 1 {
			log.Println(jobQuery.Name + " is running")
			c.JSON(200, gin.H{"message": jobQuery.Name + " is running", "done": false})
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
	go engine.cleanup()
	select {}
}
