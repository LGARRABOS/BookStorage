"""Entrée WSGI pour les déploiements en production."""

from app import app as application

# Pour compatibilité avec certains outils (gunicorn, waitress...), exposer
# également l'objet sous le nom « app ».
app = application
