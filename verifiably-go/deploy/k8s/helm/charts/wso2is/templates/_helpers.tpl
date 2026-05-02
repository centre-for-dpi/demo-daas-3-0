{{- define "wso2is.name" -}}{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}{{- end -}}

{{- define "wso2is.fullname" -}}
{{- if .Values.fullnameOverride -}}{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}{{ printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}{{- end -}}
{{- end -}}

{{- define "wso2is.labels" -}}
app.kubernetes.io/name: {{ include "wso2is.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "wso2is.selectorLabels" -}}
app.kubernetes.io/name: {{ include "wso2is.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "wso2is.host" -}}
{{- if .Values.wso2is.ingress.host -}}{{ .Values.wso2is.ingress.host }}
{{- else if .Values.wso2is.baseUrl -}}{{ .Values.wso2is.baseUrl | trimPrefix "https://" | trimPrefix "http://" | trimSuffix "/" }}
{{- else if .Values.global.domain -}}wso2.{{ .Values.global.domain }}
{{- end -}}
{{- end -}}
