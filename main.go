package main

import (
	"context"
	"flag"
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	//+kubebuilder:scaffold:imports

		snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"

	

		"kube-forensics-controller/controllers"

		"kube-forensics-controller/pkg/storage"

	

		"strings"

		"time"

	

		"gopkg.in/DataDog/dd-trace-go.v1/profiler"

	)

	

	var (

		scheme   = runtime.NewScheme()

		setupLog = ctrl.Log.WithName("setup")

	)

	

	func init() {

		utilruntime.Must(clientgoscheme.AddToScheme(scheme))

		utilruntime.Must(snapshotv1.AddToScheme(scheme))

		//+kubebuilder:scaffold:scheme

	}

	

	func main() {

		// Collector Mode

		if len(os.Args) > 1 && os.Args[1] == "collector" {

			runCollector()

			return

		}

	

		var metricsAddr string

		var enableLeaderElection bool

		var probeAddr string

		var targetNamespace string

		var forensicTTL string

		var maxLogSize int64

		var ignoreNamespaces string

		var watchNamespaces string

		var enableSecretCloning bool

		var enableCheckpointing bool

		var rateLimitWindow string

		var enableDatadogProfiling bool

		var datadogServiceName string

		var s3Bucket string

		var s3Region string

	

		flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")

		flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")

		flag.BoolVar(&enableLeaderElection, "leader-elect", false,

			"Enable leader election for controller manager. "+

				"Enabling this will ensure there is only one active controller manager.")

		flag.StringVar(&targetNamespace, "target-namespace", "debug-forensics", "The namespace where forensic pods will be created.")

		flag.StringVar(&forensicTTL, "forensic-ttl", "24h", "Time to live for forensic pods (e.g., 24h, 30m).")

		flag.Int64Var(&maxLogSize, "max-log-size", 500*1024, "Maximum log size to capture in bytes.")

		flag.StringVar(&ignoreNamespaces, "ignore-namespaces", "kube-system,kube-public", "Comma-separated list of namespaces to ignore.")

		flag.StringVar(&watchNamespaces, "watch-namespaces", "", "Comma-separated list of namespaces to watch. If empty, watches all (except ignored).")

		flag.BoolVar(&enableSecretCloning, "enable-secret-cloning", true, "Enable cloning of secrets to the forensic namespace. Security caution advised.")

		flag.BoolVar(&enableCheckpointing, "enable-checkpointing", false, "Enable experimental Container Checkpointing (requires Kubelet feature gate).")

		flag.StringVar(&rateLimitWindow, "rate-limit-window", "1h", "Window for deduplicating similar crashes (e.g., 1h, 10m).")

		

		// S3 Flags

		flag.StringVar(&s3Bucket, "s3-bucket", "", "S3 Bucket for exporting forensic artifacts (logs).")

		flag.StringVar(&s3Region, "s3-region", "us-east-1", "AWS Region for S3.")

	

		// Datadog Flags

		flag.BoolVar(&enableDatadogProfiling, "enable-datadog-profiling", false, "Enable Datadog Continuous Profiling.")

		flag.StringVar(&datadogServiceName, "datadog-service-name", "kube-forensics-controller", "Service name for Datadog.")

	

		opts := zap.Options{

			Development: false,

		}

		opts.BindFlags(flag.CommandLine)

		flag.Parse()

	

		ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	

		// Initialize S3 Exporter

		var provider storage.Provider

		if s3Bucket != "" {

			setupLog.Info("Initializing S3 Provider", "bucket", s3Bucket, "region", s3Region)

			s3Exp, err := storage.NewS3Provider(context.TODO(), s3Bucket, s3Region)

			if err != nil {

				setupLog.Error(err, "Failed to initialize S3 Provider")

				os.Exit(1)

			}

			provider = s3Exp

		} else {

			provider = &storage.NoOpProvider{}

		}

	

		// Initialize Datadog Profiler

		if enableDatadogProfiling {

			setupLog.Info("Starting Datadog Profiler", "service", datadogServiceName)

			err := profiler.Start(

				profiler.WithService(datadogServiceName),

				profiler.WithProfileTypes(

					profiler.CPUProfile,

					profiler.HeapProfile,

					profiler.GoroutineProfile,

					profiler.MutexProfile,

					profiler.BlockProfile,

				),

			)

			if err != nil {

				setupLog.Error(err, "Failed to start Datadog profiler")

				// We don't exit, just log the error

			} else {

				defer profiler.Stop()

			}

		}

	

		// Parse TTL

		ttlDuration, err := time.ParseDuration(forensicTTL)

		if err != nil {

			setupLog.Error(err, "unable to parse forensic-ttl")

			os.Exit(1)

		}

	

		// Parse Rate Limit Window

		rateLimitDuration, err := time.ParseDuration(rateLimitWindow)

		if err != nil {

			setupLog.Error(err, "unable to parse rate-limit-window")

			os.Exit(1)

		}

	

		ignoreList := strings.Split(ignoreNamespaces, ",")

		for i := range ignoreList {

			ignoreList[i] = strings.TrimSpace(ignoreList[i])

		}

		

		var watchList []string

		if watchNamespaces != "" {

			parts := strings.Split(watchNamespaces, ",")

			for _, p := range parts {

				if strings.TrimSpace(p) != "" {

					watchList = append(watchList, strings.TrimSpace(p))

				}

			}

		}

	

		config := controllers.ForensicsConfig{

			TargetNamespace:     targetNamespace,

			ForensicTTL:         ttlDuration,

			MaxLogSizeBytes:     maxLogSize,

			IgnoreNamespaces:    ignoreList,

			WatchNamespaces:     watchList,

			EnableSecretCloning: enableSecretCloning,

			EnableCheckpointing: enableCheckpointing,

			RateLimitWindow:     rateLimitDuration,

		}

	

		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{

			Scheme:                 scheme,

			Metrics:                metricsserver.Options{BindAddress: metricsAddr},

			HealthProbeBindAddress: probeAddr,

			LeaderElection:         enableLeaderElection,

			LeaderElectionID:       "forensics.k8s.io",

		})

		if err != nil {

			setupLog.Error(err, "unable to start manager")

			os.Exit(1)

		}

	

		if err = (&controllers.PodReconciler{

			Client:     mgr.GetClient(),

			Scheme:     mgr.GetScheme(),

			KubeClient: nil, // Will be initialized in SetupWithManager

			Config:     config,

			Recorder:   mgr.GetEventRecorderFor("kube-forensics-controller"),

			Storage:    provider,

		}).SetupWithManager(mgr); err != nil {

			setupLog.Error(err, "unable to create controller", "controller", "Pod")

			os.Exit(1)

		}

		//+kubebuilder:scaffold:builder

	

		if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {

			setupLog.Error(err, "unable to set up health check")

			os.Exit(1)

		}

		if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {

			setupLog.Error(err, "unable to set up ready check")

			os.Exit(1)

		}

	

		setupLog.Info("starting manager")

		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {

			setupLog.Error(err, "problem running manager")

			os.Exit(1)

		}

	}

	

	func runCollector() {

		var file string

		var bucket string

		var region string

		var key string

	

		fs := flag.NewFlagSet("collector", flag.ExitOnError)

		fs.StringVar(&file, "file", "", "Path to file to upload")

		fs.StringVar(&bucket, "s3-bucket", "", "S3 Bucket")

		fs.StringVar(&region, "s3-region", "us-east-1", "S3 Region")

		fs.StringVar(&key, "s3-key", "", "S3 Key")

		

		fs.Parse(os.Args[2:])

	

		if file == "" || bucket == "" || key == "" {

			fmt.Println("Usage: collector --file=... --s3-bucket=... --s3-key=...")

			os.Exit(1)

		}

	

		fmt.Printf("Starting collector for %s -> s3://%s/%s\n", file, bucket, key)

	

		provider, err := storage.NewS3Provider(context.Background(), bucket, region)

		if err != nil {

			fmt.Printf("Error initializing S3: %v\n", err)

			os.Exit(1)

		}

	

		url, err := provider.UploadFile(context.Background(), key, file)

		if err != nil {

			fmt.Printf("Error uploading file: %v\n", err)

			os.Exit(1)

		}

	

		fmt.Printf("Successfully uploaded: %s\n", url)

		

		// Cleanup

		if err := os.Remove(file); err != nil {

			fmt.Printf("Warning: Failed to delete local file %s: %v\n", file, err)

		} else {

			fmt.Printf("Deleted local file %s\n", file)

		}

	}
