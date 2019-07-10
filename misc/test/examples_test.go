// Copyright 2016-2017, Pulumi Corporation.  All rights reserved.

package test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	eksUtils "github.com/pulumi/pulumi-eks/utils"
	"github.com/pulumi/pulumi/pkg/testing/integration"
	"github.com/stretchr/testify/assert"
)

func TestExamples(t *testing.T) {
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = "us-west-1"
		fmt.Println("Defaulting AWS_REGION to 'us-west-1'.  You can override using the AWS_REGION environment variable")
	}
	azureEnviron := os.Getenv("ARM_ENVIRONMENT")
	if azureEnviron == "" {
		azureEnviron = "public"
		fmt.Println("Defaulting ARM_ENVIRONMENT to 'public'.  You can override using the ARM_ENVIRONMENT variable")
	}
	azureLocation := os.Getenv("ARM_LOCATION")
	if azureLocation == "" {
		azureLocation = "westus"
		fmt.Println("Defaulting ARM_LOCATION to 'westus'.  You can override using the ARM_LOCATION variable")
	}
	cwd, err := os.Getwd()
	if !assert.NoError(t, err, "expected a valid working directory: %v", err) {
		return
	}
	overrides, err := integration.DecodeMapString(os.Getenv("PULUMI_TEST_NODE_OVERRIDES"))
	if !assert.NoError(t, err, "expected valid override map: %v", err) {
		return
	}

	base := integration.ProgramTestOptions{
		Tracing:              "https://tracing.pulumi-engineering.com/collector/api/v1/spans",
		ExpectRefreshChanges: true,
		Overrides:            overrides,
		Quick:                true,
		SkipRefresh:          true,
	}

	sTests := []integration.ProgramTestOptions{
		base.With(integration.ProgramTestOptions{
			Dir: path.Join(cwd, "..", "..", "aws-ts-eks-migrate-nodegroups"),
			Config: map[string]string{
				"aws:region": awsRegion,
			},
			Dependencies: []string{
				"@pulumi/eks",
			},
			EditDirs: []integration.EditDir{
				// Add the new, 4xlarge node group
				{
					Dir:      path.Join(cwd, "..", "..", "aws-ts-eks-migrate-nodegroups", "steps", "step1"),
					Additive: true,
					ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
						maxWait := 10 * time.Minute
						endpoint := fmt.Sprintf("%s/echoserver", stack.Outputs["nginxServiceUrl"].(string))
						headers := map[string]string{
							"Host": "apps.example.com",
						}
						assertHTTPResultWithRetry(t, endpoint, headers, maxWait, func(body string) bool {
							return assert.NotEmpty(t, body, "Body should not be empty")
						})
					},
				},
				// Retarget NGINX to node select 4xlarge nodegroup, and force
				// its migration via rolling update.
				{
					Dir:      path.Join(cwd, "..", "..", "aws-ts-eks-migrate-nodegroups", "steps", "step2"),
					Additive: true,
					ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
						maxWait := 10 * time.Minute
						endpoint := fmt.Sprintf("%s/echoserver", stack.Outputs["nginxServiceUrl"].(string))
						headers := map[string]string{
							"Host": "apps.example.com",
						}
						assertHTTPResultWithRetry(t, endpoint, headers, maxWait, func(body string) bool {
							return assert.NotEmpty(t, body, "Body should not be empty")
						})

						var err error
						var out []byte
						scriptsDir := path.Join(cwd, "..", "..", "aws-ts-eks-migrate-nodegroups", "scripts")

						kubeconfig, err := json.Marshal(stack.Outputs["kubeconfig"])

						// wait for all pods across all namespaces to be ready after migration
						kubeAccess, err := eksUtils.KubeconfigToKubeAccess(kc)
						if err != nil {
							return nil, err
						}
						eksUtils.AssertAllPodsReady(t, kubeAccess.Clientset)

						// drain & delete t3.2xlarge

						// client-go instead of shell'ing out to kubectl
						if !assert.NoError(t, err, "expected kubeconfig json marshaling to not error: %v", err) {
							return
						}
						// Extract kubeconfig and write it to a temp file -
						kubeconfigFile, err := ioutil.TempFile(os.TempDir(), "kubeconfig-*.json")
						if !assert.NoError(t, err, "expected tempfile to be created: %v", err) {
							return
						}
						// Remember to clean up the file afterwards
						defer os.Remove(kubeconfigFile.Name())
						_, err = kubeconfigFile.Write(kubeconfig)
						if !assert.NoError(t, err, "expected kubeconfig to be written to tempfile with no error: %v", err) {
							return
						}
						os.Setenv("KUBECONFIG", kubeconfigFile.Name())
						defer os.Remove(kubeconfigFile.Name())
						err = kubeconfigFile.Close()
						if !assert.NoError(t, err, "expected kubeconfig file to close with no error: %v", err) {
							return
						}

						// Exec kubectl drain
						out, err = exec.Command("/bin/bash", path.Join(scriptsDir, "drain-t3.2xlarge-nodes.sh")).Output()
						if !assert.NoError(t, err, "expected no errors during kubectl drain: %v", err) {
							return
						}
						t.Logf("kubectl drain output:%s", out)

						// Exec kubectl delete
						out, err = exec.Command("/bin/bash", path.Join(scriptsDir, "delete-t3.2xlarge-nodes.sh")).Output()
						if !assert.NoError(t, err, "expected no errors during kubectl delete: %v", err) {
							return
						}
						t.Logf("kubectl delete output:%s", out)
					},
				},
				// Remove the 2xlarge node group
				{
					Dir:      path.Join(cwd, "..", "..", "aws-ts-eks-migrate-nodegroups", "steps", "step3"),
					Additive: true,
					ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
						maxWait := 10 * time.Minute
						endpoint := fmt.Sprintf("%s/echoserver", stack.Outputs["nginxServiceUrl"].(string))
						headers := map[string]string{
							"Host": "apps.example.com",
						}
						assertHTTPResultWithRetry(t, endpoint, headers, maxWait, func(body string) bool {
							return assert.NotEmpty(t, body, "Body should not be empty")
						})
					},
				},
			},
		}),
	}

	longTests := []integration.ProgramTestOptions{
		base.With(integration.ProgramTestOptions{
			Dir: path.Join(cwd, "..", "..", "azure-ts-aks-helm"),
			Config: map[string]string{
				"azure:environment": azureEnviron,
				"password":          "testTEST1234+_^$",
				"sshPublicKey":      "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDeREOgHTUgPT00PTr7iQF9JwZQ4QF1VeaLk2nHKRvWYOCiky6hDtzhmLM0k0Ib9Y7cwFbhObR+8yZpCgfSX3Hc3w2I1n6lXFpMfzr+wdbpx97N4fc1EHGUr9qT3UM1COqN6e/BEosQcMVaXSCpjqL1jeNaRDAnAS2Y3q1MFeXAvj9rwq8EHTqqAc1hW9Lq4SjSiA98STil5dGw6DWRhNtf6zs4UBy8UipKsmuXtclR0gKnoEP83ahMJOpCIjuknPZhb+HsiNjFWf+Os9U6kaS5vGrbXC8nggrVE57ow88pLCBL+3mBk1vBg6bJuLBCp2WTqRzDMhSDQ3AcWqkucGqf dremy@remthinkpad",
			},
			ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
				assertHTTPResult(t, stack.Outputs["serviceIP"], nil, func(body string) bool {
					return assert.Contains(t, body, "It works!")
				})
			},
		}),
		// TODO: This test fails due to a bug in the Terraform Azure provider in which the
		// service principal is not available when attempting to create the K8s cluster.
		// See the azure-ts-aks-multicluster readme for more detail and
		// https://github.com/terraform-providers/terraform-provider-azurerm/issues/1635.
		base.With(integration.ProgramTestOptions{
			Dir: path.Join(cwd, "..", "..", "azure-ts-aks-multicluster"),
			Config: map[string]string{
				"azure:environment": azureEnviron,
				"password":          "testTEST1234+_^$",
				"sshPublicKey":      "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDeREOgHTUgPT00PTr7iQF9JwZQ4QF1VeaLk2nHKRvWYOCiky6hDtzhmLM0k0Ib9Y7cwFbhObR+8yZpCgfSX3Hc3w2I1n6lXFpMfzr+wdbpx97N4fc1EHGUr9qT3UM1COqN6e/BEosQcMVaXSCpjqL1jeNaRDAnAS2Y3q1MFeXAvj9rwq8EHTqqAc1hW9Lq4SjSiA98STil5dGw6DWRhNtf6zs4UBy8UipKsmuXtclR0gKnoEP83ahMJOpCIjuknPZhb+HsiNjFWf+Os9U6kaS5vGrbXC8nggrVE57ow88pLCBL+3mBk1vBg6bJuLBCp2WTqRzDMhSDQ3AcWqkucGqf dremy@remthinkpad",
			},
		}),
	}

	// tests := shortTests
	tests := sTests
	if !testing.Short() {
		tests = append(tests, longTests...)
	}

	for _, ex := range tests {
		example := ex
		t.Run(filepath.Base(example.Dir), func(t *testing.T) {
			integration.ProgramTest(t, &example)
		})
	}
}

func assertHTTPResult(t *testing.T, output interface{}, headers map[string]string, check func(string) bool) bool {
	return assertHTTPResultWithRetry(t, output, headers, 5*time.Minute, check)
}

func assertHTTPResultWithRetry(t *testing.T, output interface{}, headers map[string]string, maxWait time.Duration, check func(string) bool) bool {
	hostname, ok := output.(string)
	if !assert.True(t, ok, fmt.Sprintf("expected `%s` output", output)) {
		return false
	}
	if !(strings.HasPrefix(hostname, "http://") || strings.HasPrefix(hostname, "https://")) {
		hostname = fmt.Sprintf("http://%s", hostname)
	}

	//fmt.Printf("debug00 - hostname: %s\n", hostname)

	var err error
	var resp *http.Response
	startTime := time.Now()
	count, sleep := 0, 0
	for true {
		now := time.Now()
		req, err := http.NewRequest("GET", hostname, nil)
		if !assert.NoError(t, err, "error reading request: %v", err) {
			return false
		}

		for k, v := range headers {
			// Host header cannot be set via req.Header.Set(), and must be set
			// directly.
			if strings.ToLower(k) == "host" {
				req.Host = v
				continue
			}
			req.Header.Set(k, v)
		}

		client := &http.Client{Timeout: time.Second * 10}

		resp, err = client.Do(req)
		if !assert.NoError(t, err, "error reading response: %v", err) {
			return false
		}

		/*
			// start
			fmt.Printf("debug01 - statusCode: %d\n", resp.StatusCode)
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if !assert.NoError(t, err) {
				return false
			}
			fmt.Printf("debug02 - response body: %s\n", body)
			// end
		*/

		if err == nil && resp.StatusCode == 200 {
			break
		}
		if now.Sub(startTime) >= maxWait {
			fmt.Printf("Timeout after %v. Unable to http.get %v successfully.", maxWait, hostname)
			break
		}
		count++
		// delay 10s, 20s, then 30s and stay at 30s
		if sleep > 30 {
			sleep = 30
		} else {
			sleep += 10
		}
		time.Sleep(time.Duration(sleep) * time.Second)
		fmt.Printf("Http Error: %v\n", err)
		fmt.Printf("  Retry: %v, elapsed wait: %v, max wait %v\n", count, now.Sub(startTime), maxWait)
	}
	if !assert.NoError(t, err) {
		return false
	}
	// Read the body
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if !assert.NoError(t, err) {
		return false
	}
	// Verify it matches expectations
	return check(string(body))
}

func assertHTTPHelloWorld(t *testing.T, output interface{}) bool {
	return assertHTTPResult(t, output, nil, func(s string) bool {
		return assert.Equal(t, "Hello, World!\n", s)
	})
}
