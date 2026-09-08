{{/* Image reference for a component: explicit .image or auralis-<name>. */}}
{{- define "auralis.image" -}}
{{- $ctx := .ctx -}}
{{- $name := .name -}}
{{- $img := .img -}}
{{- $repo := $img | default (printf "auralis-%s" $name) -}}
{{- printf "%s/%s:%s" $ctx.Values.image.registry $repo ($ctx.Values.image.tag | toString) -}}
{{- end -}}

{{/* Standard labels. */}}
{{- define "auralis.labels" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/part-of: auralis
app.kubernetes.io/managed-by: {{ .ctx.Release.Service }}
helm.sh/chart: {{ .ctx.Chart.Name }}-{{ .ctx.Chart.Version }}
{{- end -}}

{{/* envFrom block shared by every service container. */}}
{{- define "auralis.envFrom" -}}
- configMapRef:
    name: auralis-config
- secretRef:
    name: auralis-secrets
{{- end -}}
