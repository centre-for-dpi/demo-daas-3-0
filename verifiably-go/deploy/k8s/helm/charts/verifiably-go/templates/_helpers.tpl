{{/* Standard chart metadata helpers. */}}

{{- define "verifiably-go.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "verifiably-go.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "verifiably-go.labels" -}}
app.kubernetes.io/name: {{ include "verifiably-go.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "verifiably-go.selectorLabels" -}}
app.kubernetes.io/name: {{ include "verifiably-go.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "verifiably-go.serviceAccountName" -}}
{{- if .Values.app.serviceAccount.create -}}
{{ include "verifiably-go.fullname" . }}
{{- else -}}
default
{{- end -}}
{{- end -}}

{{/* Compute the public ingress host from baseUrl when not set. */}}
{{- define "verifiably-go.ingressHost" -}}
{{- if .Values.app.ingress.host -}}
{{ .Values.app.ingress.host }}
{{- else if .Values.app.baseUrl -}}
{{ .Values.app.baseUrl | trimPrefix "https://" | trimPrefix "http://" | trimSuffix "/" }}
{{- else if .Values.global.domain -}}
app.{{ .Values.global.domain }}
{{- end -}}
{{- end -}}
