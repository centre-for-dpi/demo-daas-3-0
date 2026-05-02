{{/* Standard chart metadata helpers. */}}

{{- define "walt-wallet.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "walt-wallet.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "walt-wallet.labels" -}}
app.kubernetes.io/name: {{ include "walt-wallet.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "walt-wallet.selectorLabels" -}}
app.kubernetes.io/name: {{ include "walt-wallet.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "walt-wallet.serviceAccountName" -}}
{{- if .Values.wallet.serviceAccount.create -}}
{{ include "walt-wallet.fullname" . }}
{{- else -}}
default
{{- end -}}
{{- end -}}

{{/* Compute the public ingress host from baseUrl when not set. */}}
{{- define "walt-wallet.ingressHost" -}}
{{- if .Values.wallet.ingress.host -}}
{{ .Values.wallet.ingress.host }}
{{- else if .Values.wallet.baseUrl -}}
{{ .Values.wallet.baseUrl | trimPrefix "https://" | trimPrefix "http://" | trimSuffix "/" }}
{{- else if .Values.global.domain -}}
wallet.{{ .Values.global.domain }}
{{- end -}}
{{- end -}}
