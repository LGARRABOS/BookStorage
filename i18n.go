package main

// Translations holds all text for both languages
type Translations map[string]string

// Lang represents supported languages
const (
	LangFR = "fr"
	LangEN = "en"
)

// DefaultLang is the default language
const DefaultLang = LangFR

// T returns translations for the given language
func T(lang string) Translations {
	if lang == LangEN {
		return translationsEN
	}
	return translationsFR
}

var translationsFR = Translations{
	// Navigation
	"nav.dashboard":    "Tableau de bord",
	"nav.stats":        "Statistiques",
	"nav.readers":      "Lecteurs",
	"nav.profile":      "Mon profil",
	"nav.admin":        "Administration",
	"nav.logout":       "Déconnexion",
	"nav.login":        "Connexion",
	"nav.register":     "Inscription",

	// Landing page
	"landing.hero.title":    "Suivez vos lectures",
	"landing.hero.subtitle": "Organisez et suivez tous vos romans, mangas, webtoons et light novels en un seul endroit.",
	"landing.hero.cta":      "Commencer gratuitement",
	"landing.hero.login":    "J'ai déjà un compte",
	"landing.features":      "Fonctionnalités",
	"landing.feature1.title": "Multi-formats",
	"landing.feature1.desc":  "Romans, mangas, manhwas, webtoons, light novels... Tous vos formats préférés.",
	"landing.feature2.title": "Suivi de progression",
	"landing.feature2.desc":  "Notez où vous en êtes dans chaque œuvre et ne perdez plus jamais le fil.",
	"landing.feature3.title": "Notes & avis",
	"landing.feature3.desc":  "Attribuez des étoiles et écrivez vos impressions personnelles.",
	"landing.feature4.title": "Statistiques",
	"landing.feature4.desc":  "Visualisez vos habitudes de lecture avec des graphiques détaillés.",
	"landing.cta.title":      "Prêt à organiser vos lectures ?",
	"landing.cta.subtitle":   "Rejoignez la communauté des lecteurs organisés.",
	"landing.cta.button":     "Créer mon compte",

	// Login page
	"login.title":       "Connexion",
	"login.username":    "Nom d'utilisateur",
	"login.password":    "Mot de passe",
	"login.submit":      "Se connecter",
	"login.no_account":  "Pas encore de compte ?",
	"login.register":    "S'inscrire",
	"login.error":       "Identifiants invalides",
	"login.pending":     "Votre compte est en attente de validation par un administrateur.",

	// Register page
	"register.title":           "Inscription",
	"register.username":        "Nom d'utilisateur",
	"register.password":        "Mot de passe",
	"register.confirm":         "Confirmer le mot de passe",
	"register.submit":          "S'inscrire",
	"register.has_account":     "Déjà un compte ?",
	"register.login":           "Se connecter",
	"register.success":         "Compte créé ! En attente de validation par un administrateur.",
	"register.error.exists":    "Ce nom d'utilisateur existe déjà",
	"register.error.mismatch":  "Les mots de passe ne correspondent pas",

	// Dashboard
	"dashboard.title":          "Mon tableau de bord",
	"dashboard.welcome":        "Bienvenue",
	"dashboard.add_work":       "Ajouter une œuvre",
	"dashboard.search":         "Rechercher...",
	"dashboard.filter.all":     "Tous",
	"dashboard.filter.type":    "Type",
	"dashboard.filter.status":  "Statut",
	"dashboard.sort":           "Trier par",
	"dashboard.sort.alpha":     "Alphabétique",
	"dashboard.sort.recent":    "Plus récent",
	"dashboard.sort.oldest":    "Plus ancien",
	"dashboard.sort.rating":    "Note",
	"dashboard.empty":          "Aucune œuvre trouvée",
	"dashboard.empty.start":    "Commencez par ajouter votre première lecture !",
	"dashboard.stats.works":    "Œuvres",
	"dashboard.stats.chapters": "Chapitres lus",
	"dashboard.stats.volumes":  "Tomes lus",
	"dashboard.chapter":        "Chapitre",
	"dashboard.volume":         "Tome",
	"dashboard.edit":           "Modifier",
	"dashboard.delete":         "Supprimer",
	"dashboard.delete.confirm": "Êtes-vous sûr de vouloir supprimer cette œuvre ?",

	// Work types
	"type.manga":       "Manga",
	"type.novel":       "Roman",
	"type.webtoon":     "Webtoon",
	"type.manhwa":      "Manhwa",
	"type.manhua":      "Manhua",
	"type.light_novel": "Light Novel",
	"type.comic":       "Comic",
	"type.other":       "Autre",

	// Work status
	"status.reading":    "En cours",
	"status.completed":  "Terminé",
	"status.on_hold":    "En pause",
	"status.dropped":    "Abandonné",
	"status.plan_to_read": "À lire",

	// Add/Edit work
	"work.add.title":       "Ajouter une œuvre",
	"work.edit.title":      "Modifier l'œuvre",
	"work.form.title":      "Titre",
	"work.form.type":       "Type",
	"work.form.status":     "Statut",
	"work.form.chapter":    "Chapitre actuel",
	"work.form.volume":     "Tome actuel",
	"work.form.image":      "Image de couverture",
	"work.form.image.current": "Image actuelle",
	"work.form.image.change": "Changer l'image",
	"work.form.rating":     "Note",
	"work.form.notes":      "Notes personnelles",
	"work.form.notes.placeholder": "Vos impressions, commentaires...",
	"work.form.submit.add": "Ajouter",
	"work.form.submit.edit": "Enregistrer",
	"work.form.cancel":     "Annuler",
	"work.form.delete":     "Supprimer",
	"work.section.info":    "Informations principales",
	"work.section.media":   "Média",
	"work.section.rating":  "Appréciation",

	// Profile
	"profile.title":        "Mon profil",
	"profile.username":     "Nom d'utilisateur",
	"profile.joined":       "Membre depuis",
	"profile.works":        "œuvres",
	"profile.avatar":       "Avatar",
	"profile.avatar.change": "Changer l'avatar",
	"profile.visibility":   "Visibilité du profil",
	"profile.public":       "Public",
	"profile.private":      "Privé",
	"profile.public.desc":  "Les autres utilisateurs peuvent voir votre bibliothèque",
	"profile.private.desc": "Votre bibliothèque est cachée des autres utilisateurs",
	"profile.password":     "Changer le mot de passe",
	"profile.password.current": "Mot de passe actuel",
	"profile.password.new": "Nouveau mot de passe",
	"profile.password.confirm": "Confirmer",
	"profile.save":         "Enregistrer",
	"profile.section.general": "Général",
	"profile.section.avatar": "Avatar",
	"profile.section.visibility": "Visibilité",
	"profile.section.security": "Sécurité",

	// Users / Readers
	"users.title":          "Lecteurs",
	"users.search":         "Rechercher un lecteur...",
	"users.empty":          "Aucun lecteur trouvé",
	"users.works":          "œuvres",
	"users.view":           "Voir la bibliothèque",
	"users.private":        "Profil privé",

	// User detail
	"user.library":         "Bibliothèque de",
	"user.empty":           "Cette bibliothèque est vide",
	"user.import":          "Importer",
	"user.import.success":  "Œuvre importée avec succès !",

	// Stats
	"stats.title":          "Statistiques",
	"stats.total_works":    "Total des œuvres",
	"stats.total_chapters": "Chapitres lus",
	"stats.total_volumes":  "Tomes lus",
	"stats.avg_rating":     "Note moyenne",
	"stats.by_type":        "Par type",
	"stats.by_status":      "Par statut",
	"stats.top_rated":      "Mieux notées",
	"stats.recent":         "Ajoutées récemment",

	// Admin
	"admin.title":          "Administration",
	"admin.accounts":       "Gestion des comptes",
	"admin.pending":        "En attente",
	"admin.approved":       "Approuvés",
	"admin.username":       "Utilisateur",
	"admin.status":         "Statut",
	"admin.role":           "Rôle",
	"admin.actions":        "Actions",
	"admin.approve":        "Approuver",
	"admin.delete":         "Supprimer",
	"admin.promote":        "Promouvoir admin",
	"admin.demote":         "Rétrograder",
	"admin.role.admin":     "Admin",
	"admin.role.user":      "Utilisateur",
	"admin.role.owner":     "Propriétaire",
	"admin.no_action":      "Aucune action",
	"admin.restricted":     "Restreint",

	// Common
	"common.save":          "Enregistrer",
	"common.cancel":        "Annuler",
	"common.delete":        "Supprimer",
	"common.edit":          "Modifier",
	"common.back":          "Retour",
	"common.loading":       "Chargement...",
	"common.error":         "Erreur",
	"common.success":       "Succès",
	"common.confirm":       "Confirmer",
	"common.yes":           "Oui",
	"common.no":            "Non",
}

var translationsEN = Translations{
	// Navigation
	"nav.dashboard":    "Dashboard",
	"nav.stats":        "Statistics",
	"nav.readers":      "Readers",
	"nav.profile":      "My Profile",
	"nav.admin":        "Administration",
	"nav.logout":       "Logout",
	"nav.login":        "Login",
	"nav.register":     "Register",

	// Landing page
	"landing.hero.title":    "Track your reading",
	"landing.hero.subtitle": "Organize and track all your novels, manga, webtoons and light novels in one place.",
	"landing.hero.cta":      "Get started for free",
	"landing.hero.login":    "I already have an account",
	"landing.features":      "Features",
	"landing.feature1.title": "Multi-format",
	"landing.feature1.desc":  "Novels, manga, manhwa, webtoons, light novels... All your favorite formats.",
	"landing.feature2.title": "Progress tracking",
	"landing.feature2.desc":  "Note where you are in each work and never lose track again.",
	"landing.feature3.title": "Notes & reviews",
	"landing.feature3.desc":  "Give star ratings and write your personal impressions.",
	"landing.feature4.title": "Statistics",
	"landing.feature4.desc":  "Visualize your reading habits with detailed charts.",
	"landing.cta.title":      "Ready to organize your reading?",
	"landing.cta.subtitle":   "Join the community of organized readers.",
	"landing.cta.button":     "Create my account",

	// Login page
	"login.title":       "Login",
	"login.username":    "Username",
	"login.password":    "Password",
	"login.submit":      "Sign in",
	"login.no_account":  "Don't have an account yet?",
	"login.register":    "Register",
	"login.error":       "Invalid credentials",
	"login.pending":     "Your account is pending administrator approval.",

	// Register page
	"register.title":           "Register",
	"register.username":        "Username",
	"register.password":        "Password",
	"register.confirm":         "Confirm password",
	"register.submit":          "Register",
	"register.has_account":     "Already have an account?",
	"register.login":           "Sign in",
	"register.success":         "Account created! Pending administrator approval.",
	"register.error.exists":    "This username already exists",
	"register.error.mismatch":  "Passwords do not match",

	// Dashboard
	"dashboard.title":          "My Dashboard",
	"dashboard.welcome":        "Welcome",
	"dashboard.add_work":       "Add a work",
	"dashboard.search":         "Search...",
	"dashboard.filter.all":     "All",
	"dashboard.filter.type":    "Type",
	"dashboard.filter.status":  "Status",
	"dashboard.sort":           "Sort by",
	"dashboard.sort.alpha":     "Alphabetical",
	"dashboard.sort.recent":    "Most recent",
	"dashboard.sort.oldest":    "Oldest",
	"dashboard.sort.rating":    "Rating",
	"dashboard.empty":          "No works found",
	"dashboard.empty.start":    "Start by adding your first reading!",
	"dashboard.stats.works":    "Works",
	"dashboard.stats.chapters": "Chapters read",
	"dashboard.stats.volumes":  "Volumes read",
	"dashboard.chapter":        "Chapter",
	"dashboard.volume":         "Volume",
	"dashboard.edit":           "Edit",
	"dashboard.delete":         "Delete",
	"dashboard.delete.confirm": "Are you sure you want to delete this work?",

	// Work types
	"type.manga":       "Manga",
	"type.novel":       "Novel",
	"type.webtoon":     "Webtoon",
	"type.manhwa":      "Manhwa",
	"type.manhua":      "Manhua",
	"type.light_novel": "Light Novel",
	"type.comic":       "Comic",
	"type.other":       "Other",

	// Work status
	"status.reading":    "Reading",
	"status.completed":  "Completed",
	"status.on_hold":    "On Hold",
	"status.dropped":    "Dropped",
	"status.plan_to_read": "Plan to Read",

	// Add/Edit work
	"work.add.title":       "Add a work",
	"work.edit.title":      "Edit work",
	"work.form.title":      "Title",
	"work.form.type":       "Type",
	"work.form.status":     "Status",
	"work.form.chapter":    "Current chapter",
	"work.form.volume":     "Current volume",
	"work.form.image":      "Cover image",
	"work.form.image.current": "Current image",
	"work.form.image.change": "Change image",
	"work.form.rating":     "Rating",
	"work.form.notes":      "Personal notes",
	"work.form.notes.placeholder": "Your impressions, comments...",
	"work.form.submit.add": "Add",
	"work.form.submit.edit": "Save",
	"work.form.cancel":     "Cancel",
	"work.form.delete":     "Delete",
	"work.section.info":    "Main information",
	"work.section.media":   "Media",
	"work.section.rating":  "Rating",

	// Profile
	"profile.title":        "My Profile",
	"profile.username":     "Username",
	"profile.joined":       "Member since",
	"profile.works":        "works",
	"profile.avatar":       "Avatar",
	"profile.avatar.change": "Change avatar",
	"profile.visibility":   "Profile visibility",
	"profile.public":       "Public",
	"profile.private":      "Private",
	"profile.public.desc":  "Other users can see your library",
	"profile.private.desc": "Your library is hidden from other users",
	"profile.password":     "Change password",
	"profile.password.current": "Current password",
	"profile.password.new": "New password",
	"profile.password.confirm": "Confirm",
	"profile.save":         "Save",
	"profile.section.general": "General",
	"profile.section.avatar": "Avatar",
	"profile.section.visibility": "Visibility",
	"profile.section.security": "Security",

	// Users / Readers
	"users.title":          "Readers",
	"users.search":         "Search for a reader...",
	"users.empty":          "No readers found",
	"users.works":          "works",
	"users.view":           "View library",
	"users.private":        "Private profile",

	// User detail
	"user.library":         "Library of",
	"user.empty":           "This library is empty",
	"user.import":          "Import",
	"user.import.success":  "Work imported successfully!",

	// Stats
	"stats.title":          "Statistics",
	"stats.total_works":    "Total works",
	"stats.total_chapters": "Chapters read",
	"stats.total_volumes":  "Volumes read",
	"stats.avg_rating":     "Average rating",
	"stats.by_type":        "By type",
	"stats.by_status":      "By status",
	"stats.top_rated":      "Top rated",
	"stats.recent":         "Recently added",

	// Admin
	"admin.title":          "Administration",
	"admin.accounts":       "Account management",
	"admin.pending":        "Pending",
	"admin.approved":       "Approved",
	"admin.username":       "Username",
	"admin.status":         "Status",
	"admin.role":           "Role",
	"admin.actions":        "Actions",
	"admin.approve":        "Approve",
	"admin.delete":         "Delete",
	"admin.promote":        "Promote to admin",
	"admin.demote":         "Demote",
	"admin.role.admin":     "Admin",
	"admin.role.user":      "User",
	"admin.role.owner":     "Owner",
	"admin.no_action":      "No action",
	"admin.restricted":     "Restricted",

	// Common
	"common.save":          "Save",
	"common.cancel":        "Cancel",
	"common.delete":        "Delete",
	"common.edit":          "Edit",
	"common.back":          "Back",
	"common.loading":       "Loading...",
	"common.error":         "Error",
	"common.success":       "Success",
	"common.confirm":       "Confirm",
	"common.yes":           "Yes",
	"common.no":            "No",
}
