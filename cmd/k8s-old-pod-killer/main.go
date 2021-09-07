package main

import (
	"context"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type TargetInfo struct {
	//Should be deployment/statefulset/daemonset
	Kind         TargetKind    `yaml:"kind"`
	NameSpace    string        `yaml:"name_space"`
	Name         string        `yaml:"name"`
	MaxLife      time.Duration `yaml:"max_life"`
	Interval     time.Duration `yaml:"interval"`
	BatchMaxKill int64         `yaml:"batch_max_kill"`
}

type Config struct {
	Dryrun          bool          `yaml:"dryrun"`
	BatchMode       bool          `yaml:"batch_mode"`
	DefaultInterval time.Duration `yaml:"default_interval"`
	Targets         []TargetInfo  `yaml:"targets"`
}

type TargetKind string

func (t TargetKind) ToLower() TargetKind {
	return TargetKind(strings.ToLower(string(t)))
}

const (
	DAEMONSET   TargetKind = "daemonset"
	DEPLOYMENT  TargetKind = "deployment"
	STATEFULSET TargetKind = "statefulset"
)

func getDaemonsetPodSelector(namespace string, name string) *metav1.LabelSelector {
	targetDaemonset, err := clientset.AppsV1().DaemonSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil
	}

	return targetDaemonset.Spec.Selector
}

func getDeploymentPodSelector(namespace string, name string) *metav1.LabelSelector {
	targetDeployment, err := clientset.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil
	}

	return targetDeployment.Spec.Selector
}

func getStatefulsetPodSelector(namespace string, name string) *metav1.LabelSelector {
	targetStatefulset, err := clientset.AppsV1().StatefulSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil
	}

	return targetStatefulset.Spec.Selector
}

var (
	clientset *kubernetes.Clientset
	wg        sync.WaitGroup
)

func performCheckAndKill(targetInfo TargetInfo, dryrun bool, batchMode bool) {
	defer func() {
		log.Printf("Loop for %s %s/%s stopped.", targetInfo.NameSpace, targetInfo.Kind, targetInfo.Name)
		wg.Done()
	}()
	var targetLabel *metav1.LabelSelector
	log.Printf("Loop for %s %s/%s started. Will try to kill pod older than %s, interval %s.", targetInfo.NameSpace, targetInfo.Kind, targetInfo.Name, targetInfo.MaxLife, targetInfo.Interval)

	for {
		// Keep fetch label everytime. Just in case label has been updated when long run.
		switch targetInfo.Kind.ToLower() {
		case DAEMONSET:
			targetLabel = getDaemonsetPodSelector(targetInfo.NameSpace, targetInfo.Name)
		case DEPLOYMENT:
			targetLabel = getDeploymentPodSelector(targetInfo.NameSpace, targetInfo.Name)
		case STATEFULSET:
			targetLabel = getStatefulsetPodSelector(targetInfo.NameSpace, targetInfo.Name)
		default:
			log.Printf("%s is not a supported kind.\n", targetInfo.Kind)
			return
		}

		if targetLabel == nil {
			log.Printf("%s %s/%s selector is nil.\n", targetInfo.NameSpace, targetInfo.Kind, targetInfo.Name)
			return
		}

		targetLabelSelector, err := metav1.LabelSelectorAsSelector(targetLabel)
		if err != nil {
			log.Println("Get label selector error: ", err.Error())
			return
		}

		var killCount int64
		pods, err := clientset.CoreV1().Pods(targetInfo.NameSpace).List(context.Background(), metav1.ListOptions{LabelSelector: targetLabelSelector.String()})
		for _, pod := range pods.Items {
			if pod.Status.Phase != "Running" {
				continue
			}
			podAge := time.Since(pod.Status.StartTime.Time)
			if podAge > targetInfo.MaxLife {
				if !dryrun {
					err = clientset.CoreV1().Pods(pod.Namespace).Evict(context.Background(), &v1beta1.Eviction{
						TypeMeta:      pod.TypeMeta,
						ObjectMeta:    pod.ObjectMeta,
						DeleteOptions: &metav1.DeleteOptions{},
					})
					if err != nil {
						log.Printf("Can't delete pod '%s', error: %s\n", pod.Name, err.Error())
						// Assume this batch can't delete any more. So wait for next batch. No need to make redundant calls
						break
					}
				} else {
					log.Println("Dryrun mode on, not actually delete anything.")
				}
				log.Printf("Deleted Pod '%s' from '%s'\n", pod.Name, pod.Namespace)

				killCount++
				if targetInfo.BatchMaxKill > 0 && killCount >= targetInfo.BatchMaxKill {
					break
				}
			}
		}

		log.Printf("This loop for %s %s/%s killed %d pod(s).", targetInfo.NameSpace, targetInfo.Kind, targetInfo.Name, killCount)

		if batchMode {
			log.Printf("Batch mode enabled, exit after run.")
			return
		}

		time.Sleep(targetInfo.Interval)
	}
}

func getConfig() *Config {
	config := &Config{}
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/config/config.yaml"
	}
	configFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatalln("Read config file error: ", err.Error())
	}
	err = yaml.Unmarshal(configFile, config)
	if err != nil {
		log.Fatalln("Parse config file error: ", err.Error())
	}

	if config.DefaultInterval < 10*time.Second {
		config.DefaultInterval = 10 * time.Second
	}

	for i := range config.Targets {
		if config.Targets[i].MaxLife < 10*time.Second {
			config.Targets[i].MaxLife = 10 * time.Second
		}
		if config.Targets[i].Interval < 10*time.Second {
			config.Targets[i].Interval = config.DefaultInterval
		}
	}

	return config
}

func main() {
	config := getConfig()

	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Get in cluster k8sConfig error: %s\n\n", err.Error())
		log.Println("Will try connect to local 8001 (kubectl proxy)")
		k8sConfig = &rest.Config{
			Host: "http://127.0.0.1:8001",
		}
	}
	clientset, err = kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		panic(err)
	}

	for _, targetInfo := range config.Targets {
		wg.Add(1)
		go performCheckAndKill(targetInfo, config.Dryrun, config.BatchMode)
	}
	wg.Wait()
}
