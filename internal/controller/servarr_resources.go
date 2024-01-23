package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/graytonio/pirate-ship/api/v1alpha1"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ServarrApplication interface {
	client.Object
	GetAppName() string
	GetDBNames() []string
	GetStatus() *v1alpha1.ServarrStatus
	GetSpec() *v1alpha1.ServarrSpec
}

const (
	// typeAvailableServarr represents the status of the Deployment reconciliation
	typeAvailableServarr = "Available"
	// typeDegradedRadarr represents the status used when the custom resource is deleted and the finalizer operations are must to occur.
	typeDegradedServarr = "Degraded"
)

func reconcileServarrCRD(c client.Client, ctx context.Context, req ctrl.Request, app ServarrApplication) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	err := c.Get(ctx, req.NamespacedName, app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("%s resource not found. Ignoring since object must be deleted", app.GetAppName()))
			return ctrl.Result{}, nil
		}
		log.Error(err, fmt.Sprintf("Failed to get %s", app.GetAppName()))
		return ctrl.Result{}, err
	}

	if app.GetStatus().Conditions == nil || len(app.GetStatus().Conditions) == 0 {
		meta.SetStatusCondition(&app.GetStatus().Conditions, metav1.Condition{Type: typeAvailableServarr, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = c.Status().Update(ctx, app); err != nil {
			log.Error(err, "Failed to update Radarr status")
			return ctrl.Result{}, err
		}
		if err := c.Get(ctx, req.NamespacedName, app); err != nil {
			log.Error(err, "Failed to re-fetch radarr")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func imageForServarr(app ServarrApplication) string {
	return fmt.Sprintf("lscr.io/linuxserver/%s:%s", app.GetAppName(), app.GetSpec().Version)
}

func labelsForServarr(app ServarrApplication) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       app.GetAppName(),
		"app.kubernetes.io/instance":   app.GetName(),
		"app.kubernetes.io/version":    app.GetSpec().Version,
		"app.kubernetes.io/part-of":    "pirate-ship",
		"app.kubernetes.io/created-by": "controller-manager",
	}
}

func deploymentForServarr(c client.Client, app ServarrApplication) (*appsv1.Deployment, error) {
	ls := labelsForServarr(app)
	replicas := app.GetSpec().Size

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.GetName(),
			Namespace: app.GetNamespace(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:           imageForServarr(app),
						Name:            app.GetAppName(),
						ImagePullPolicy: corev1.PullIfNotPresent,
						Env: []corev1.EnvVar{
							{
								Name:  "PUID",
								Value: "1000",
							},
							{
								Name:  "PGID",
								Value: "1000",
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "config-xml",
								MountPath: "/config/config.xml",
								SubPath:   "config.xml",
							},
						},
						Ports: []corev1.ContainerPort{{
							ContainerPort: app.GetSpec().ContainerPort,
							Name:          "radarr",
						}},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "config-xml",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config", app.GetName()),
									},
									Items: []corev1.KeyToPath{
										{
											Key:  "config.xml",
											Path: "config.xml",
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

	if err := ctrl.SetControllerReference(app, dep, c.Scheme()); err != nil {
		return nil, err
	}

	return dep, nil
}

func reconcileServarrDeployment(c client.Client, ctx context.Context, req ctrl.Request, app ServarrApplication) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	found := &appsv1.Deployment{}
	err := c.Get(ctx, types.NamespacedName{Name: app.GetName(), Namespace: app.GetNamespace()}, found)

	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		dep, err := deploymentForServarr(c, app)
		if err != nil {
			log.Error(err, "Failed to define new Deployment resource for Radarr")

			// The following implementation will update the status
			meta.SetStatusCondition(&app.GetStatus().Conditions, metav1.Condition{Type: typeAvailableServarr, // TODO figure out types
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", app.GetName(), err)})

			if err := c.Status().Update(ctx, app); err != nil {
				log.Error(err, "Failed to update Radarr status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		log.Info("Creating a new Deployment",
			"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		if err = c.Create(ctx, dep); err != nil {
			log.Error(err, "Failed to create new Deployment",
				"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		// Deployment created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	size := app.GetSpec().Size
	if *found.Spec.Replicas != size {
		found.Spec.Replicas = &size
		if err = c.Update(ctx, found); err != nil {
			log.Error(err, "Failed to update Deployment",
				"Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)

			// Re-fetch the radarr Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := c.Get(ctx, req.NamespacedName, app); err != nil {
				log.Error(err, "Failed to re-fetch radarr")
				return ctrl.Result{}, err
			}

			// The following implementation will update the status
			meta.SetStatusCondition(&app.GetStatus().Conditions, metav1.Condition{Type: typeAvailableServarr,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", app.GetName(), err)})

			if err := c.Status().Update(ctx, app); err != nil {
				log.Error(err, "Failed to update Radarr status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		// Now, that we update the size we want to requeue the reconciliation
		// so that we can ensure that we have the latest state of the resource before
		// update. Also, it will help ensure the desired state on the cluster
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

func generateConfigXML(app ServarrApplication) (string, error) {
	templateString := `
	<Config>
		<BindAddress>*</BindAddress>
		<Port>7878</Port>
		<SslPort>9898</SslPort>
		<EnableSsl>False</EnableSsl>
		<LaunchBrowser>True</LaunchBrowser>
		<ApiKey>{{ .APIKey }}</ApiKey>
		<AuthenticationMethod>Basic</AuthenticationMethod>
		<AuthenticationRequired>DisabledForLocalAddresses</AuthenticationRequired>
		<Branch>master</Branch>
		<LogLevel>info</LogLevel>
		<SslCertPath></SslCertPath>
		<SslCertPassword></SslCertPassword>
		<UrlBase></UrlBase>
		<InstanceName>{{ .Name }}</InstanceName>
		<UpdateMechanism>Docker</UpdateMechanism>
		<PostgresUser>{{ .DBUser }}</PostgresUser>
		<PostgresPassword>{{ .DBPassword }}</PostgresPassword>
		<PostgresPort>{{ .DBPort }}</PostgresPort>
		<PostgresHost>{{ .DBHost }}</PostgresHost>
	</Config>`

	type ConfigParms struct {
		Name       string
		APIKey     string
		DBUser     string
		DBPort     int
		DBHost     string
		DBPassword string
	}

	tmpl, err := template.New("radarr-config.xml").Parse(templateString)
	if err != nil {
		return "", err
	}

	// TODO Generate API Key
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, ConfigParms{
		Name:       app.GetName(),
		APIKey:     "1131852c83ea4cb7ab6c175a823480e6",
		DBUser:     app.GetSpec().DB.Credentials.Username.Value,
		DBPassword: app.GetSpec().DB.Credentials.Password.Value,
		DBHost:     app.GetSpec().DB.Host,
		DBPort:     app.GetSpec().DB.Port,
	})
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func configMapForServarr(c client.Client, app ServarrApplication) (*corev1.ConfigMap, error) {
	configData, err := generateConfigXML(app)
	if err != nil {
		return nil, err
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", app.GetName()),
			Namespace: app.GetNamespace(),
		},
		Data: map[string]string{
			"config.xml": configData,
		},
	}

	if err := ctrl.SetControllerReference(app, cm, c.Scheme()); err != nil {
		return nil, err
	}
	return cm, nil
}

func reconcileServarrConfigMap(c client.Client, ctx context.Context, req ctrl.Request, app ServarrApplication) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	found := &corev1.ConfigMap{}
	err := c.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-config", app.GetName()), Namespace: app.GetNamespace()}, found)
	if err != nil && apierrors.IsNotFound(err) {
		cm, err := configMapForServarr(c, app)
		if err != nil {
			log.Error(err, "Failed to define new ConfigMap resource for Radarr")

			// The following implementation will update the status
			meta.SetStatusCondition(&app.GetStatus().Conditions, metav1.Condition{Type: typeAvailableServarr,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create ConfigMap for the custom resource (%s): (%s)", app.GetName(), err)})

			if err := c.Status().Update(ctx, app); err != nil {
				log.Error(err, "Failed to update Radarr status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		log.Info("Creating a new ConfigMap",
			"ConfigMap.Namespace", cm.Namespace, "ConfigMap.Name", cm.Name)
		if err = c.Create(ctx, cm); err != nil {
			log.Error(err, "Failed to create new Deployment",
				"ConfigMap.Namespace", cm.Namespace, "ConfigMap.Name", cm.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get ConfigMap")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	currentData, err := generateConfigXML(app)
	if err != nil {
		log.Error(err, "Invalid DB Config Spec")
		return ctrl.Result{}, err
	}

	if found.Data["config.xml"] != currentData {
		found.Data["config.xml"] = currentData
		if err = c.Update(ctx, found); err != nil {
			log.Error(err, "Failed to update ConfigMap",
				"ConfigMap.Namespace", found.Namespace, "ConfigMap.Name", found.Name)

			// Re-fetch the radarr Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := c.Get(ctx, req.NamespacedName, app); err != nil {
				log.Error(err, "Failed to re-fetch radarr")
				return ctrl.Result{}, err
			}

			// The following implementation will update the status
			meta.SetStatusCondition(&app.GetStatus().Conditions, metav1.Condition{Type: typeAvailableServarr,
				Status: metav1.ConditionFalse, Reason: "Updating Config",
				Message: fmt.Sprintf("Failed to update the config for the custom resource (%s): (%s)", app.GetName(), err)})

			if err := c.Status().Update(ctx, app); err != nil {
				log.Error(err, "Failed to update Radarr status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		// Now, that we update the size we want to requeue the reconciliation
		// so that we can ensure that we have the latest state of the resource before
		// update. Also, it will help ensure the desired state on the cluster
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

func reconcileServarrDB(c client.Client, ctx context.Context, req ctrl.Request, app ServarrApplication) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Creating db connection")

	connString := fmt.Sprintf("postgres://%s:%s@%s:%d", app.GetSpec().DB.Credentials.Username.Value, app.GetSpec().DB.Credentials.Password.Value, app.GetSpec().DB.Host, app.GetSpec().DB.Port)
	db, err := pgx.Connect(ctx, connString)
	if err != nil {
		log.Error(err, "could not connect to app db")
		return ctrl.Result{}, err
	}
	defer db.Close(ctx)

	// Create dbs
	for _, dbName := range app.GetDBNames() {
		_, err := db.Exec(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, dbName))
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code != "42P04" { // Ignore db already exits errors
				log.Error(err, "could not create database for app")
				return ctrl.Result{}, err
			}
		}
	}

	// TODO Check if tables exist for db before applying config

	return ctrl.Result{}, nil
}
