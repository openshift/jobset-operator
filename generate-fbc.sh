#!/usr/bin/env bash

declare -A TECH_PREVIEW_VERSIONS
TECH_PREVIEW_VERSIONS[v4.18]="true"
TECH_PREVIEW_VERSIONS[v4.19]="true"
TECH_PREVIEW_VERSIONS[v4.20]="true"
TECH_PREVIEW_VERSIONS[v4.21]="true"

for OCP_VERSION in  v4.18 v4.19 v4.20 v4.21 v4.22; do
    echo "OCP_VERSION: ${OCP_VERSION}"
    opm alpha render-template semver $OCP_VERSION/catalog-template.yaml --migrate-level=bundle-object-to-csv-metadata > $OCP_VERSION/catalog/jobset-operator/catalog.json
    if [[ "${TECH_PREVIEW_VERSIONS[${OCP_VERSION}]}" == "true" ]]; then
      # preserve the old channel
      sed -i "s/stable-v0.1/tech-preview-v0.1/g" $OCP_VERSION/catalog/jobset-operator/catalog.json
    fi
done
