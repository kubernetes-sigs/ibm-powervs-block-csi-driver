package e2e

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/onsi/ginkgo/config"
	"k8s.io/apimachinery/pkg/util/uuid"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/onsi/ginkgo/reporters"
	"k8s.io/kubernetes/test/e2e/framework"
	frameworkconfig "k8s.io/kubernetes/test/e2e/framework/config"
)

const kubeconfigEnvVar = "KUBECONFIG"

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
	testing.Init()
	// k8s.io/kubernetes/test/e2e/framework requires env KUBECONFIG to be set
	// it does not fall back to defaults
	if os.Getenv(kubeconfigEnvVar) == "" {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		os.Setenv(kubeconfigEnvVar, kubeconfig)
	}
	fmt.Println(os.Getenv(kubeconfigEnvVar))
	framework.AfterReadingAllFlags(&framework.TestContext)

	frameworkconfig.CopyFlags(frameworkconfig.Flags, flag.CommandLine)
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)
	flag.Parse()
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)

	// Run tests through the Ginkgo runner with output to console + JUnit for Jenkins
	var r []Reporter
	if framework.TestContext.ReportDir != "" {
		if err := os.MkdirAll(framework.TestContext.ReportDir, 0755); err != nil {
			log.Fatalf("Failed creating report directory: %v", err)
		} else {
			r = append(r, reporters.NewJUnitReporter(path.Join(framework.TestContext.ReportDir, fmt.Sprintf("junit_%v%02d.xml", framework.TestContext.ReportPrefix, config.GinkgoConfig.ParallelNode))))
		}
	}
	log.Printf("Starting e2e run %q on Ginkgo node %d", uuid.NewUUID(), config.GinkgoConfig.ParallelNode) // TODO use framework.RunID like upstream

	RunSpecsWithDefaultAndCustomReporters(t, "Power VS CSI Driver End-to-End Tests", r)
}
