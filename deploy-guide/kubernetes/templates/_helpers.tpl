{{/*
Validates the values that would otherwise only fail at runtime (the Funnel
server refuses to start, or tasks fail when their storage is created).
*/}}
{{- define "funnel.validate" -}}
{{- if not (has .Values.pvcMode (list "shared" "pvc" "full")) -}}
{{- fail (printf "pvcMode must be one of shared, pvc, full (got %q)" .Values.pvcMode) -}}
{{- end -}}
{{- if and (eq .Values.pvcMode "pvc") (not (or .Values.storageClassName .Values.pvc.storageClass)) -}}
{{- fail "pvcMode \"pvc\" needs storageClassName (or pvc.storageClass)" -}}
{{- end -}}
{{- if and (eq .Values.pvcMode "full") (not (and .Values.s3.bucket .Values.s3.region)) -}}
{{- fail "pvcMode \"full\" needs s3.bucket and s3.region" -}}
{{- end -}}
{{- end -}}
