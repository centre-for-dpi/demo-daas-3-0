{{/* Standard chart metadata helpers. */}}

{{- define "walt-issuer.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "walt-issuer.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "walt-issuer.labels" -}}
app.kubernetes.io/name: {{ include "walt-issuer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "walt-issuer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "walt-issuer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "walt-issuer.serviceAccountName" -}}
{{- if .Values.issuer.serviceAccount.create -}}
{{ include "walt-issuer.fullname" . }}
{{- else -}}
default
{{- end -}}
{{- end -}}

{{/* Compute the public ingress host from baseUrl when not set. */}}
{{- define "walt-issuer.ingressHost" -}}
{{- if .Values.issuer.ingress.host -}}
{{ .Values.issuer.ingress.host }}
{{- else if .Values.issuer.baseUrl -}}
{{ .Values.issuer.baseUrl | trimPrefix "https://" | trimPrefix "http://" | trimSuffix "/" }}
{{- else if .Values.global.domain -}}
issuer.{{ .Values.global.domain }}
{{- end -}}
{{- end -}}
