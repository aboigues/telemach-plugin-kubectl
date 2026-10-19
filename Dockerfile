# Image telemachlearning/telemach-kubectl : essayer TELEMACH sans installation (« check ») ou
# installer le plugin kubectl sur la machine hôte (« install »).
#
# Le contexte est préparé par GoReleaser (dockers_v2) : les binaires linux de la plateforme
# visée sous linux/<arch>/, et les dossiers de compilation darwin de dist/ (extra_files).
# Aucune compilation ici, aucun shell ni gestionnaire de paquets dans l'image (scratch),
# utilisateur non-root, aucun kubeconfig : le plugin ne peut contacter aucun cluster.
FROM scratch

ARG TARGETPLATFORM
ARG TARGETARCH

COPY $TARGETPLATFORM/telemach-image /usr/local/bin/telemach-image
COPY $TARGETPLATFORM/kubectl-telemach /opt/telemach/linux/kubectl-telemach
COPY dist/kubectl-telemach_darwin_${TARGETARCH}_*/kubectl-telemach /opt/telemach/darwin/kubectl-telemach

# Dossier des manifestes monté pour l'essai (« check » sans -f le vérifie).
WORKDIR /manifestes
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/telemach-image"]
