for OCP_VERSION in v4.18 v4.19 v4.20; do
    echo "OCP_VERSION: ${OCP_VERSION}"
    opm alpha render-template semver $OCP_VERSION/catalog-template.yaml --migrate-level=bundle-object-to-csv-metadata > $OCP_VERSION/catalog/jobset-operator/catalog.json;
done
