/*
Copyright 2021 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconciler

import (
	"context"
	"fmt"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/logging"
	kreconciler "knative.dev/pkg/reconciler"
)

type Reconciler struct {
	EnqueueAfter func(interface{}, time.Duration)
}

// ReconcileKind implements Interface.ReconcileKind.
func (c *Reconciler) ReconcileKind(ctx context.Context, r *v1alpha1.Run) kreconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Infof("Reconciling %s/%s", r.Namespace, r.Name)

	// Ignore completed waits.
	if r.IsDone() {
		logger.Info("Run is finished, done reconciling")
		return nil
	}

	if r.Spec.Ref == nil ||
		r.Spec.Ref.APIVersion != "jenkins.tekton.dev/v0" || r.Spec.Ref.Kind != "JenkinsJob" {
		// This is not a Run we should have been notified about; do nothing.
		return nil
	}
	if r.Spec.Ref.Name != "" {
		r.Status.MarkRunFailed("UnexpectedName", "unexpected ref name: %s", r.Spec.Ref.Name)
		return nil
	}

	expr := r.Spec.GetParam("url")
	if expr == nil || expr.Value.StringVal == "" {
		r.Status.MarkRunFailed("MissingURL", "url param was not passed")
		return nil
	}

	if len(r.Spec.Params) != 1 {
		var found []string
		for _, p := range r.Spec.Params {
			if p.Name == "url" {
				continue
			}
			found = append(found, p.Name)
		}
		r.Status.MarkRunFailed("UnexpectedParams", "unexpected params: %v", found)
		return nil
	}

	url :=  expr.Value.StringVal
	if r.Status.StartTime == nil {
		now := metav1.Now()
		r.Status.StartTime = &now
		resp, err := http.Post(url+"/build", "application/json", nil)
		if err != nil {
			r.Status.MarkRunFailed("UnexpectedErrorPOST", "unexpected error: %v", err)
			return nil
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			r.Status.MarkRunFailed("UnexpectedErrorPOST", "unexpected error: %v", err)
			return nil
		}
		r.Status.Results = append(r.Status.Results,v1alpha1.RunResult{
			Name: "job-status",
			Value: fmt.Sprintf("%s", body),
		})
		defer resp.Body.Close()
		r.Status.MarkRunRunning("Waiting", "waiting for job to complete")
	}

	done := false
	if r.Status.StartTime != nil {
		resp, err := http.Get(url)
		if err != nil {
			r.Status.MarkRunFailed("UnexpectedErrorGET", "found unexpected error: %v", err)
			return nil
		}
		defer resp.Body.Close()
	}

	if done {
		now := metav1.Now()
		r.Status.CompletionTime = &now
		r.Status.MarkRunSucceeded("JenkinsJobCompleted", "jenkins job has finished executing")
	} else {
		// Enqueue another check when the timeout should be elapsed.
		c.EnqueueAfter(r, time.Until(r.Status.StartTime.Time.Add(5000)))
	}

	return kreconciler.NewEvent(corev1.EventTypeNormal, "RunReconciled", "Run reconciled: \"%s/%s\"", r.Namespace, r.Name)
}
