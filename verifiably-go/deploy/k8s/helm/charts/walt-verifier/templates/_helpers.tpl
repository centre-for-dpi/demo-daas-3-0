{{/* Standard chart metadata helpers. */}}

{{- define "walt-verifier.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "walt-verifier.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "walt-verifier.labels" -}}
app.kubernetes.io/name: {{ include "walt-verifier.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "walt-verifier.selectorLabels" -}}
app.kubernetes.io/name: {{ include "walt-verifier.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "walt-verifier.serviceAccountName" -}}
{{- if .Values.verifier.serviceAccount.create -}}
{{ include "walt-verifier.fullname" . }}
{{- else -}}
default
{{- end -}}
{{- end -}}

{{/* Compute the public ingress host from baseUrl when not set. */}}
{{- define "walt-verifier.ingressHost" -}}
{{- if .Values.verifier.ingress.host -}}
{{ .Values.verifier.ingress.host }}
{{- else if .Values.verifier.baseUrl -}}
{{ .Values.verifier.baseUrl | trimPrefix "https://" | trimPrefix "http://" | trimSuffix "/" }}
{{- else if .Values.global.domain -}}
verifier.{{ .Values.global.domain }}
{{- end -}}
{{- end -}}
