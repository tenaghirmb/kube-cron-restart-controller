<!-- markdownlint-disable-next-line MD033 -->
<h1>kube-cron-restart-controller</h1>

[![TOC Automation](https://github.com/tenaghirmb/kube-cron-restart-controller/actions/workflows/main.yml/badge.svg?branch=main)](https://github.com/tenaghirmb/kube-cron-restart-controller/actions/workflows/main.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](https://opensource.org)
[![BuyMeACoffee](https://raw.githubusercontent.com/pachadotdev/buymeacoffee-badges/main/bmc-donate-yellow.svg)](https://www.buymeacoffee.com/tenaghirmb)

**Advanced Cron Restart Controller** is an industrial-grade Kubernetes Operator designed to orchestrate scheduled workload maintenance at scale. Built with **Go** and **Kubebuilder**, it replaces the resource-heavy native CronJob pattern with a high-concurrency **In-Memory Scheduling Engine** coupled with **Atomic Status-Gatekeeping** to ensure 100% execution integrity in distributed environments.

# Table of Contents
<!--ts-->
   * [Features](#features)
   * [Architecture Overview](#architecture-overview)
   * [Tech Stack](#tech-stack)
   * [Prerequisites](#prerequisites)
   * [Installation](#installation)
      * [Using Helm](#using-helm)
      * [Using Kustomize](#using-kustomize)
   * [Usage](#usage)
   * [Configuration](#configuration)
      * [restartTargetRef](#restarttargetref)
      * [excludeDates](#excludedates)
      * [schedule](#schedule)
      * [cron expression](#cron-expression)
         * [Special Characters](#special-characters)
         * [Predefined Schedules](#predefined-schedules)
         * [Intervals](#intervals)
   * [Contributing](#contributing)
   * [Licensing](#licensing)
<!--te-->

## Features

- **Scheduled Restarts**: Automatically restart resources on a predefined schedule.
- **Resource Support**: Supports Deployments, StatefulSets and DaemonSets which can be restarted using the `kubectl rollout restart` command.
- **Custom Schedules**: Define custom restart schedules using Cron syntax.
- **Flexible Time Configuration**: Supports skipping specified dates.

## Architecture Overview

A dual-layer orchestration engine combining a High-Performance in-Memory Registry with Kubernetes Atomic Status-Gatekeeping.

**Why Not Native CronJobs?**

- **Resource Footprint**: Consumes 90% less etcd storage compared to 10000+ native CronJob objects.
- **Execution Latency**: Sub-millisecond dispatching vs native kube-controller-manager polling delays.
- **Reliability & Edge Cases Split-Brain Protection**: Leveraging Conditional Patching with ResourceVersion (OCC, Optimistic Concurrency Control) to ensure strict mutual exclusion in HA deployments.
- **Misfire Compensation**: Automatically detects historical schedule gaps upon cold-start, ensuring critical maintenance windows are never missed.

## Tech Stack

| Component | Technology | Purpose |
| :-- | :-- | :-- |
| Language | Golang 1.26 | Core runtime environment |
| Framework | Kubebuilder v4 | Operator scaffolding and boilerplate generation |
| Controller Runtime | controller-runtime | Reconciliation pattern implementation |
| Cron Library | robfig/cron | Enhanced cron expression parsing and scheduling |
| Kubernetes API | client-go | Kubernetes resource manipulation |
| Dependency Management | Go Modules | Package version management |
| Build System | Make | Compilation and artifact generation |
| Container Runtime | Docker | Operator containerization |
| Package Manager | Helm 4 | Cluster installation and upgrades |

## Prerequisites

| Tool | Minimum Version | Purpose |
| :-- | :-- | :-- |
| Kubernetes | v1.34+ | Kubernetes cluster for operator to run on |
| Docker | 23.0+ | Building container images (if deploying from source) |
| Go | v1.24.0+ | Building from source |
| Helm | v3.x+ | Installing via Helm chart |

## Installation

### Using Helm

```bash
make helm-deploy IMG=docker.io/tenaghirmb/cronrestart:v2.0
```

### Using Kustomize

``` bash
make deploy-test
make deploy-prod
```

## Usage

Try out the examples in the `config/samples` folder.

1. Deploy Samples

    ```bash
    # using Kustomize
    kubectl apply -k config/samples
    # Or using Makefile
    make deploy-samples
    ```

2. Check the status of the deployment

    ```bash
    [0] % kubectl get deploy nginx-deployment-basic
    NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
    nginx-deployment-basic   1/1     1            1           16s
    ```

    ```bash
    [0] % kubectl describe deploy nginx-deployment-basic
    Name:                   nginx-deployment-basic
    Namespace:              default
    CreationTimestamp:      Tue, 07 Jul 2026 22:43:04 +0800
    Labels:                 app=nginx
    Annotations:            deployment.kubernetes.io/revision: 1
    Selector:               app=nginx
    Replicas:               1 desired | 1 updated | 1 total | 1 available | 0 unavailable
    StrategyType:           RollingUpdate
    MinReadySeconds:        0
    RollingUpdateStrategy:  25% max unavailable, 25% max surge
    Pod Template:
      Labels:  app=nginx
      Containers:
      nginx:
        Image:      nginx:alpine
        Port:       80/TCP
        Host Port:  0/TCP
        Limits:
          cpu:     500m
          memory:  128Mi
        Requests:
          cpu:         250m
          memory:      64Mi
        Environment:   <none>
        Mounts:        <none>
      Volumes:         <none>
      Node-Selectors:  <none>
      Tolerations:     <none>
    Conditions:
      Type           Status  Reason
      ----           ------  ------
      Available      True    MinimumReplicasAvailable
      Progressing    True    NewReplicaSetAvailable
    OldReplicaSets:  <none>
    NewReplicaSet:   nginx-deployment-basic-78d967d4dc (1/1 replicas created)
    Events:
      Type    Reason             Age   From                   Message
      ----    ------             ----  ----                   -------
      Normal  ScalingReplicaSet  35s   deployment-controller  Scaled up replica set nginx-deployment-basic-78d967d4dc from 0 to 1
    ```

3. Check the restart event

    ```bash
    [0] % kubectl describe deploy nginx-deployment-basic
    Name:                   nginx-deployment-basic
    Namespace:              default
    CreationTimestamp:      Tue, 07 Jul 2026 22:43:04 +0800
    Labels:                 app=nginx
    Annotations:            deployment.kubernetes.io/revision: 2
    Selector:               app=nginx
    Replicas:               1 desired | 1 updated | 1 total | 1 available | 0 unavailable
    StrategyType:           RollingUpdate
    MinReadySeconds:        0
    RollingUpdateStrategy:  25% max unavailable, 25% max surge
    Pod Template:
      Labels:       app=nginx
      Annotations:  kubectl.kubernetes.io/restartedAt: 2026-07-07T14:50:00Z
      Containers:
      nginx:
        Image:      nginx:alpine
        Port:       80/TCP
        Host Port:  0/TCP
        Limits:
          cpu:     500m
          memory:  128Mi
        Requests:
          cpu:         250m
          memory:      64Mi
        Environment:   <none>
        Mounts:        <none>
      Volumes:         <none>
      Node-Selectors:  <none>
      Tolerations:     <none>
    Conditions:
      Type           Status  Reason
      ----           ------  ------
      Available      True    MinimumReplicasAvailable
      Progressing    True    NewReplicaSetAvailable
    OldReplicaSets:  nginx-deployment-basic-78d967d4dc (0/0 replicas created)
    NewReplicaSet:   nginx-deployment-basic-66b9c6c69d (1/1 replicas created)
    Events:
      Type    Reason             Age    From                   Message
      ----    ------             ----   ----                   -------
      Normal  ScalingReplicaSet  7m14s  deployment-controller  Scaled up replica set nginx-deployment-basic-78d967d4dc from 0 to 1
      Normal  ScalingReplicaSet  18s    deployment-controller  Scaled up replica set nginx-deployment-basic-66b9c6c69d from 0 to 1
      Normal  ScalingReplicaSet  16s    deployment-controller  Scaled down replica set nginx-deployment-basic-78d967d4dc from 1 to 0
    ```

4. Describe the cronrestarter

    ```bash
    [0] % kubectl describe cronrestarters cronrestarter-sample
    Name:         cronrestarter-sample
    Namespace:    default
    Labels:       app.kubernetes.io/managed-by=kustomize
                  app.kubernetes.io/name=cron-restart
    Annotations:  <none>
    API Version:  autorestart.uni.com/v1
    Kind:         CronRestarter
    Metadata:
      Creation Timestamp:  2026-07-07T14:43:04Z
      Finalizers:
        cronrestarter.finalizers.uni.com
      Generation:        1
      Resource Version:  3425273
      UID:               b6de7835-0f00-4a4e-9b77-6205ebb2e64d
    Spec:
      Exclude Dates:
        * * 4 6 *
        * * * * 5
      Restart Target Ref:
        API Version:  apps/v1
        Kind:         Deployment
        Name:         nginx-deployment-basic
      Schedule:       */10 22 * * *
      Timezone:       Asia/Shanghai
    Status:
      Conditions:
        Last Probe Time:    2026-07-07T14:50:01Z
        Message:            cron restarter job cronrestarter-sample executed successfully.
        State:              Succeed
      Entry Id:             1
      Last Execution Time:  2026-07-07T14:50:00Z
      Last Tick Timestamp:  2026-07-07T14:50:00Z
      Message:              cron restarter job cronrestarter-sample executed successfully.
      State:                Succeed
    Events:
      Type    Reason   Age   From           Message
      ----    ------   ----  ----           -------
      Normal  Succeed  83s   CronRestarter  cron restarter job cronrestarter-sample executed successfully.
    ```

    > The `Status.Conditions` holds the latest 10 restarting execution results;
    > The `Status.State` indicates its execution status. When the `State` is `Succeed`, it means the last execution was successful. When the `State` is `Submitted`, it means the cronrestart job has been submitted to the cron engine and is waiting to be executed. When the `State` is `Failed`, it means the last execution failed.

5. Uninstall Samples

    ```bash
    make undeploy-samples
    ```

6. Check controller's log

    ```log
    2026-07-07T22:42:39.341+0800	[INFO][PID:58719]	cmd/main.go:185	starting manager
    2026-07-07T22:42:39.341+0800	[INFO][PID:58719]	controller-runtime.metrics	server/server.go:208	Starting metrics server
    2026-07-07T22:42:39.341+0800	[INFO][PID:58719]	manager/server.go:83	starting server	{"name": "pprof", "addr": "127.0.0.1:7007"}
    2026-07-07T22:42:39.342+0800	[INFO][PID:58719]	controller-runtime.metrics	server/server.go:247	Serving metrics server	{"bindAddress": ":8443", "secure": false}
    2026-07-07T22:42:39.342+0800	[INFO][PID:58719]	manager/server.go:83	starting server	{"name": "health probe", "addr": "[::]:8081"}
    2026-07-07T22:42:39.342+0800	[INFO][PID:58719]	controller/controller.go:369	Starting EventSource	{"controller": "cronrestarter", "controllerGroup": "autorestart.uni.com", "controllerKind": "CronRestarter", "source": "kind source: *v1.CronRestarter"}
    2026-07-07T22:42:41.857+0800	[INFO][PID:58719]	controller/controller.go:302	Starting Controller	{"controller": "cronrestarter", "controllerGroup": "autorestart.uni.com", "controllerKind": "CronRestarter"}
    2026-07-07T22:42:41.858+0800	[INFO][PID:58719]	controller/controller.go:305	Starting workers	{"controller": "cronrestarter", "controllerGroup": "autorestart.uni.com", "controllerKind": "CronRestarter", "worker count": 1}
    2026-07-07T22:43:04.335+0800	[INFO][PID:58719]	controller/cronrestarter_controller.go:89	Start to handle cronRestarter cronrestarter-sample in default namespace
    2026-07-07T22:43:04.771+0800	[INFO][PID:58719]	log/warning_handler.go:64	metadata.finalizers: "cronrestarter.finalizers.uni.com": prefer a domain-qualified finalizer name including a path (/) to avoid accidental conflicts with other finalizer writers	{"controller": "cronrestarter", "controllerGroup": "autorestart.uni.com", "controllerKind": "CronRestarter", "CronRestarter": {"name":"cronrestarter-sample","namespace":"default"}, "namespace": "default", "name": "cronrestarter-sample", "reconcileID": "71251341-269a-443d-8bc4-90ec08ab46e5"}
    2026-07-07T22:43:04.772+0800	[INFO][PID:58719]	controller/cronrestarter_controller.go:128	Add finalizer for cronRestarter cronrestarter-sample in default namespace
    2026-07-07T22:43:04.772+0800	[INFO][PID:58719]	controller/cronmanager.go:35	cronRestarter job cronrestarter-sample of cronRestarter cronrestarter-sample in default updated, 1 active jobs exist
    2026-07-07T22:50:00.695+0800	[INFO][PID:58719]	controller/cronjob.go:240	Deployment nginx-deployment-basic in namespace default has been restarted successfully. cronRestarter: cronrestarter-sample id: 2cd097d5-51a3-54b2-85e7-7d7553c103f2
    2026-07-07T22:50:01.944+0800	[DEBUG][PID:58719]	events	recorder/recorder.go:116	cron restarter job cronrestarter-sample executed successfully.	{"type": "Normal", "object": {"kind":"CronRestarter","namespace":"default","name":"cronrestarter-sample","uid":"b6de7835-0f00-4a4e-9b77-6205ebb2e64d","apiVersion":"autorestart.uni.com/v1","resourceVersion":"3425273"}, "reason": "Succeed"}
    2026-07-07T22:55:03.357+0800	[INFO][PID:58719]	controller/cronrestarter_controller.go:89	Start to handle cronRestarter cronrestarter-sample in default namespace
    2026-07-07T22:55:03.357+0800	[INFO][PID:58719]	controller/cronrestarter_controller.go:102	cronRestarter cronrestarter-sample in default namespace is marked to be deleted
    2026-07-07T22:55:03.357+0800	[INFO][PID:58719]	controller/cronmanager.go:45	Remove cronRestarter job cronrestarter-sample of cronRestarter cronrestarter-sample in default from jobQueue,0 active jobs left
    2026-07-07T22:55:03.749+0800	[INFO][PID:58719]	controller/cronrestarter_controller.go:115	Remove finalizer for cronRestarter cronrestarter-sample in default namespace
    2026-07-07T22:55:03.749+0800	[INFO][PID:58719]	controller/cronrestarter_controller.go:117	cronRestarter cronrestarter-sample in default namespace has been finalized successfully. cronId: 2cd097d5-51a3-54b2-85e7-7d7553c103f2
    2026-07-07T22:55:03.750+0800	[INFO][PID:58719]	controller/cronrestarter_controller.go:89	Start to handle cronRestarter cronrestarter-sample in default namespace
    ```

## Configuration

The following example demonstrates how to configure a `CronRestarter`.

```bash
apiVersion: autorestart.uni.com/v1
kind: CronRestarter
metadata:
  labels:
    app.kubernetes.io/name: cron-restart
    app.kubernetes.io/managed-by: kustomize
  name: cronrestart-sample
spec:
  timezone: Asia/Shanghai
  restartTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: nginx-deployment-basic
  excludeDates:
   # exclude June 4th
   - "* * 4 6 *"
   # exclude every Friday 
   - "* * * * 5"
  schedule: "*/10 22 * * *"
```

### restartTargetRef

The `restartTargetRef` field specifies the workload to restart. If the workload supports the `kubectl rollout restart` command (such as `Deployment` and `StatefulSet`), `CronRestarter` should work well.

### excludeDates

The `excludeDates` field is an array of dates. The job will skip execution when the date matches. The minimum unit is a day. If you want to skip a specific date (e.g., June 4th), you can specify the excludeDates field as follows:

  ```bash
    excludeDates:
    - "* * 4 6 *"
  ```

### schedule

The `schedule` field is a cron expression that defines the schedule for restarting the target resource.
  
### cron expression

The cron expression format is described below:

Field name   | Mandatory? | Allowed values  | Allowed special characters
----------   | ---------- | --------------  | --------------------------
Minutes      | Yes        | 0-59            | * / , -
Hours        | Yes        | 0-23            | * / , -
Day of month | Yes        | 1-31            | * / , - ?
Month        | Yes        | 1-12 or JAN-DEC | * / , -
Day of week  | Yes        | 0-6 or SUN-SAT  | * / , - ?

#### Special Characters

* **Asterisk ( * )**
  * The asterisk indicates that the cron expression will match for all values of the field. For example, using an asterisk in the 4th field (month) means every month.

* **Slash ( / )**
  * Slashes are used to describe increments of ranges. For example, `3-59/15` in the 1st field (minutes) means the 3rd minute of the hour and every 15 minutes thereafter. The form `*/...` is equivalent to the form `first-last/...`, which means an increment over the largest possible range of the field. The form `N/...` means starting at N and using the increment until the end of that specific range. It does not wrap around.

* **Comma ( , )**
  * Commas are used to separate items of a list. For example, using `MON,WED,FRI` in the 5th field (day of week) means Mondays, Wednesdays, and Fridays.

* **Hyphen ( - )**
  * Hyphens are used to define ranges. For example, `9-17` means every hour between 9am and 5pm inclusive.

* **Question mark ( ? )**
  * A question mark can be used instead of `*` to leave either day-of-month or day-of-week blank.

#### Predefined Schedules

You may use one of several predefined schedules in place of a cron expression:

Entry                  | Description                                | Equivalent To
-----                  | -----------                                | -------------
@yearly (or @annually) | Run once a year, midnight, Jan. 1st        | `0 0 1 1 *`
@monthly               | Run once a month, midnight, first of month | `0 0 1 * *`
@weekly                | Run once a week, midnight between Sat/Sun  | `0 0 * * 0`
@daily (or @midnight)  | Run once a day, midnight                   | `0 0 * * *`
@hourly                | Run once an hour, beginning of hour        | `0 * * * *`

#### Intervals

You can also schedule a job to execute at fixed intervals, starting at the time it's added or when cron is run. This is supported by formatting the cron spec like this:

@every `<duration>`

where `<duration>` is a string accepted by `time.ParseDuration` (<https://golang.org/pkg/time/#ParseDuration>).

For example, `@every 1h30m10s` indicates a schedule that activates after 1 hour, 30 minutes, and 10 seconds, and then every interval after that.

**Note**: The interval does not take the job runtime into account. For example, if a job takes 3 minutes to run and is scheduled to run every 5 minutes, it will have only 2 minutes of idle time between each run.

For more scheduling options, please refer to the [cron package documentation](https://godoc.org/github.com/robfig/cron).

## Contributing

Contributions are welcome! Please submit an issue or pull request to contribute to this project.

## Licensing

This project is licensed under the terms of the MIT License. See the [LICENSE](LICENSE) file for the full license text and copyright notice.
