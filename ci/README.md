# Tekton CI Pipeline

This directory contains the configuration for a tekton pipeline that tests
the installed `route-monitor-operator` version in the logged in cluster.

## Installation

A `route-monitor-operator` installation has to be present.
The rest of the installation is done by running the following commands:

First, apply the subscription to the pipeline operator: 
```console
$ oc apply -f pipeline-operator-subscription.yaml
```
Wait a minute until it becomes available then apply the rest:
```console
$ oc apply -f . 
```

The pipeline and other resources will be installed in the `ci` namespace.

## Configuration

The main configuration lies in `trigger.yaml`.

* The repository URL is first configured in the `TriggerBinding`. The git revision
will be automatically overwritten by the installed RMO version
* Next, an `EventListener` is set up that listens on a port within the cluster for
post requests to trigger a pipeline execution
(http://el-pipeline-event-listener.ci.svc.cluster.local:8080)
* Further, a `CronJob` is configured to periodically curl the event listener

## Triggering a Pipeline Run

A pipeline will be triggered automatically. However, a run can be started via the GUI. Make sure to use the `Start last run` option, because otherwise a wrong service account will be utilized. A run can also be triggered via the console:

```console
$ oc create -f pipeline-run.yaml
```

The logs of the last pipeline can be fetched with the command as long as the pods are still available:

```console
$ tkn pipelinerun logs -f -n ci $(tkn pipelinerun list -n ci -o name --limit 1 | cut -d "/" -f2)
```

The result of the last runs can be seen with:

```console
$ tkn pipelinerun list -n ci 
```

The documentation for further tekton commands is available [here](https://docs.openshift.com/container-platform/4.4/cli_reference/tkn_cli/op-tkn-reference.html).
