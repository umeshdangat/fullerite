package podinfo

import (
	"encoding/json"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"net/http"
	"time"
)

const podSpecURL = "http://localhost:10255/pods"

// PodInfoParser Used to retrieve selected corev1.Pod objects 
type PodInfoParser struct {
	kubeletTimeout int
	selectors      []Selector
}

// NewPodInfoParser Return a new PodInfoParser
func NewPodInfoParser(kubeletTimeout int, selectors ...Selector) *PodInfoParser {
	podInfoParser := &PodInfoParser{
		kubeletTimeout: 5,
		selectors:      make([]Selector, 0),
	}
	for _, selector := range selectors {
		podInfoParser.selectors = append(podInfoParser.selectors, selector)
	}
	return podInfoParser
}

// AllPods Return all unfiltered pods on this host. This information is 
// retrieved from podSpecURL 
func (p *PodInfoParser) AllPods() ([]corev1.Pod, error) {
	client := http.Client{
		Timeout: time.Second * time.Duration(p.kubeletTimeout),
	}

	res, getErr := client.Get(podSpecURL)
	if getErr != nil {
		return nil, getErr
	}
	defer res.Body.Close()
	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}
	podList := corev1.PodList{}
	jsonErr := json.Unmarshal(body, &podList)
	if jsonErr != nil {
		return nil, jsonErr
	}
	return podList.Items, nil
}

// Pods Return pods filterd by provided selector argument, and selectors 
// in this PodInfoParser object
func (p *PodInfoParser) Pods(selectors ...Selector) ([]corev1.Pod, error) {
	pods, err := p.AllPods()
	if err != nil {
		return nil, err
	}
	return p.SelectPodsFrom(pods, selectors...)
}

// Pods Return pods filterd by provided selector argument, and selectors 
// in this PodInfoParser object
func (p *PodInfoParser) SelectPodsFrom(pods []corev1.Pod, selectors ...Selector) ([]corev1.Pod, error) {
	ret := make([]corev1.Pod, 0)
	for _, pod := range pods {
		isPodLegit := true
		for _, s := range p.selectors {
			if !isPodLegit {
				break
			}
			if !s.Filter(&pod) {
				isPodLegit = false
			}
		}
		for _, s := range selectors {
			if !isPodLegit {
				break
			}
			if !s.Filter(&pod) {
				isPodLegit = false
			}
		}
		if isPodLegit {
			ret = append(ret, pod)
		}
	}
	return ret, nil
}