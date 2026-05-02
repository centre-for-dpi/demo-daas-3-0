{{- define "libretranslate.name" -}}{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}{{- end -}}

{{- define "libretranslate.fullname" -}}
{{- if .Values.fullnameOverride -}}{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}{{ printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}{{- end -}}
{{- end -}}

{{- define "libretranslate.labels" -}}
app.kubernetes.io/name: {{ include "libretranslate.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "libretranslate.selectorLabels" -}}
app.kubernetes.io/name: {{ include "libretranslate.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
