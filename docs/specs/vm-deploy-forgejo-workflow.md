# Deploy HealthVault from Forgejo main

Idea: [Idea 1130](https://ideaforge.ikoro.in/idea/1130)

## Why

Use the common home-side VM deployer after a trusted merge into `main`.
Keep the VM independent of the home network.
The owner approves Runner installation and HealthVault production activation on 2026-10-16.

## How

Route only `push` events for `main` to the repository-scoped `vm-deploy-trusted` Runner.
Call its root-controlled `/opt/vm-deploy/ci.py` entry point with `healthvault`.
Pass the event identity and exact source revision through environment variables.
Let the entry point fetch its own checkout and reject superseded revisions.
Build images at home through bounded Portainer, not on the VM.
Require a fresh encrypted project-local backup before replacing application containers.
Do not wait for the NAS download.
Do not automatically roll back images, configuration or the database.
Stop only the failed deployment attempt's new containers.
Notify Telegram about detected deployment errors and workflow interruptions.
A dead Runner cannot send its own notification.
Keep KinCart, Android and home Portainer deployments outside this change.
Keep credentials and activation settings outside repository workflows.
Do not add repository checkout actions, deployment hooks or PR jobs.

Validate the workflow contract locally.
Track live Runner, first production deployment and actual merge-event acceptance in Idea 1130.
Do not run the application's mutating E2E suite against production.
Merge this PR only with the owner's explicit merge approval.

## Validation Commands

```sh
make test-workflows
git diff --check
```

### Task 1: Connect the trusted deployment workflow

- [x] Add a main-push-only workflow for HealthVault.
- [x] Add an interruption finalizer that sends a notification without deploying.
- [x] Test the workflow trigger, runner, fixed commands and event identity contract.
- [x] Run the Review Gate.
- [x] Mark completed.
