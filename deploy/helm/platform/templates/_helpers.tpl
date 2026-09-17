{{- define "platform.fullname" -}}
{{- .Release.Name }}
{{- end -}}

{{- define "platform.labels" -}}
app.kubernetes.io/part-of: talos-platform
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "platform.secretName" -}}
{{- if .Values.secrets.existingSecret -}}
{{ .Values.secrets.existingSecret }}
{{- else -}}
{{ include "platform.fullname" . }}-secrets
{{- end -}}
{{- end -}}
