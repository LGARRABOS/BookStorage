import binascii
import hmac
import os
import sqlite3
import uuid
from functools import wraps
from hashlib import scrypt

from flask import Flask, render_template, request, redirect, url_for, session, flash, jsonify
from werkzeug.security import generate_password_hash, check_password_hash
from werkzeug.utils import secure_filename

app = Flask(__name__)
app.secret_key = "votre_cle_secrete"  # Remplacez par une clé plus sûre

DEFAULT_DATABASE = os.environ.get("BOOKSTORAGE_DATABASE", "database.db")
app.config.setdefault("DATABASE", DEFAULT_DATABASE)
DEFAULT_SUPERADMIN_USERNAME = os.environ.get("BOOKSTORAGE_SUPERADMIN_USERNAME", "superadmin")
DEFAULT_SUPERADMIN_PASSWORD = os.environ.get("BOOKSTORAGE_SUPERADMIN_PASSWORD", "SuperAdmin!2023")
UPLOAD_FOLDER = os.path.join("static", "images")
PROFILE_UPLOAD_FOLDER = os.path.join("static", "avatars")
app.config["UPLOAD_FOLDER"] = UPLOAD_FOLDER
app.config["PROFILE_UPLOAD_FOLDER"] = PROFILE_UPLOAD_FOLDER
os.makedirs(os.path.join(app.root_path, UPLOAD_FOLDER), exist_ok=True)
os.makedirs(os.path.join(app.root_path, PROFILE_UPLOAD_FOLDER), exist_ok=True)
ALLOWED_EXTENSIONS = {"png", "jpg", "jpeg", "gif"}

READING_TYPES = [
    "Roman",
    "Manga",
    "BD",
    "Manhwa",
    "Light Novel",
    "Comics",
    "Webtoon",
    "Essai",
    "Poésie",
    "Autre",
]


def _resolve_media_path(stored_path, config_key):
    if not stored_path:
        return None

    filename = os.path.basename(stored_path)
    if not filename:
        return None

    storage_dir = app.config.get(config_key)
    if not storage_dir:
        return None

    if not os.path.isabs(storage_dir):
        storage_dir = os.path.join(app.root_path, storage_dir)

    return os.path.join(storage_dir, filename)


def _delete_media_file(stored_path, config_key):
    file_path = _resolve_media_path(stored_path, config_key)
    if file_path and os.path.exists(file_path):
        try:
            os.remove(file_path)
        except OSError:
            pass

CREATE_USERS_TABLE_SQL = """
    CREATE TABLE IF NOT EXISTS users (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        username TEXT UNIQUE NOT NULL,
        password TEXT NOT NULL,
        validated INTEGER DEFAULT 0,
        is_admin INTEGER DEFAULT 0,
        is_superadmin INTEGER DEFAULT 0,
        display_name TEXT,
        email TEXT,
        bio TEXT,
        avatar_path TEXT,
        is_public INTEGER DEFAULT 1
    );
"""

CREATE_WORKS_TABLE_SQL = """
    CREATE TABLE IF NOT EXISTS works (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        title TEXT NOT NULL,
        chapter INTEGER DEFAULT 0,
        link TEXT,
        status TEXT,
        image_path TEXT,
        reading_type TEXT,
        user_id INTEGER NOT NULL,
        FOREIGN KEY (user_id) REFERENCES users (id)
    );
"""


def verify_password(stored_hash: str, password: str) -> bool:
    """Validate a password against Werkzeug hashes, including legacy scrypt ones."""

    try:
        return check_password_hash(stored_hash, password)
    except ValueError as exc:
        if not stored_hash or not stored_hash.startswith("scrypt:"):
            raise exc

        try:
            method, salt, hash_value = stored_hash.split("$", 2)
            _, n_str, r_str, p_str = method.split(":", 3)
            n, r, p = int(n_str), int(r_str), int(p_str)
        except ValueError:
            raise exc

        try:
            derived = scrypt(
                password.encode("utf-8"),
                salt=salt.encode("utf-8"),
                n=n,
                r=r,
                p=p,
                maxmem=64 * 1024 * 1024,
            )
        except ValueError:
            raise exc
        derived_hex = binascii.hexlify(derived).decode("ascii")
        return hmac.compare_digest(derived_hex, hash_value)


def allowed_file(filename):
    return '.' in filename and filename.rsplit('.', 1)[1].lower() in ALLOWED_EXTENSIONS

PROFILE_COLUMNS = {
    "display_name": "TEXT",
    "email": "TEXT",
    "bio": "TEXT",
    "avatar_path": "TEXT",
    "is_public": "INTEGER DEFAULT 1",
}

WORK_COLUMNS = {
    "reading_type": "TEXT",
}


def ensure_profile_columns(conn):
    existing_columns = {
        column_info[1] for column_info in conn.execute("PRAGMA table_info(users)").fetchall()
    }
    missing = {name: column_type for name, column_type in PROFILE_COLUMNS.items() if name not in existing_columns}
    for column_name, column_type in missing.items():
        conn.execute(f"ALTER TABLE users ADD COLUMN {column_name} {column_type}")
    if missing:
        conn.commit()


def ensure_work_columns(conn):
    existing_columns = {
        column_info[1] for column_info in conn.execute("PRAGMA table_info(works)").fetchall()
    }
    missing = {name: column_type for name, column_type in WORK_COLUMNS.items() if name not in existing_columns}
    for column_name, column_type in missing.items():
        conn.execute(f"ALTER TABLE works ADD COLUMN {column_name} {column_type}")
    if missing:
        conn.commit()


def ensure_super_admin(conn):
    existing = conn.execute(
        "SELECT 1 FROM users WHERE is_superadmin = 1 LIMIT 1"
    ).fetchone()
    if existing:
        return

    cursor = conn.execute(
        "UPDATE users SET validated = 1, is_admin = 1, is_superadmin = 1 WHERE username = ?",
        (DEFAULT_SUPERADMIN_USERNAME,),
    )
    if cursor.rowcount:
        conn.commit()
        return

    hashed_password = generate_password_hash(
        DEFAULT_SUPERADMIN_PASSWORD, method="pbkdf2:sha256"
    )
    conn.execute(
        "INSERT INTO users (username, password, validated, is_admin, is_superadmin) VALUES (?, ?, 1, 1, 1)",
        (DEFAULT_SUPERADMIN_USERNAME, hashed_password),
    )
    conn.commit()


def ensure_schema(conn):
    conn.execute("PRAGMA foreign_keys = ON;")
    conn.execute(CREATE_USERS_TABLE_SQL)
    conn.execute(CREATE_WORKS_TABLE_SQL)
    ensure_profile_columns(conn)
    ensure_work_columns(conn)
    ensure_super_admin(conn)


def get_db_connection():
    conn = sqlite3.connect(app.config["DATABASE"])
    conn.row_factory = sqlite3.Row
    try:
        ensure_schema(conn)
    except sqlite3.OperationalError:
        # La table peut ne pas encore exister lors de l'initialisation.
        pass
    return conn


def bootstrap_database():
    conn = None
    try:
        conn = get_db_connection()
    finally:
        if conn is not None:
            conn.close()


bootstrap_database()

# Décorateur pour forcer l'accès aux administrateurs
def admin_required(func):
    @wraps(func)
    def wrapper(*args, **kwargs):
        if "user_id" not in session:
            flash("Veuillez vous connecter.")
            return redirect(url_for("login"))
        conn = get_db_connection()
        user = conn.execute("SELECT * FROM users WHERE id = ?", (session["user_id"],)).fetchone()
        if not user:
            conn.close()
            session.clear()
            flash("Votre session n'est plus valide. Veuillez vous reconnecter.")
            return redirect(url_for("login"))

        is_admin = bool(user["is_admin"])
        is_superadmin = bool(user["is_superadmin"])
        session["is_admin"] = is_admin
        session["is_superadmin"] = is_superadmin
        conn.close()

        if not is_admin:
            flash("Accès réservé aux administrateurs.")
            return redirect(url_for("dashboard"))
        return func(*args, **kwargs)
    return wrapper

# Route d'inscription : les nouveaux utilisateurs sont créés avec validated=0 et is_admin=0
@app.route("/register", methods=["GET", "POST"])
def register():
    if request.method == "POST":
        username = request.form["username"]
        password = request.form["password"]
        hashed_password = generate_password_hash(password, method="pbkdf2:sha256")
        conn = get_db_connection()
        try:
            conn.execute(
                "INSERT INTO users (username, password, validated, is_admin) VALUES (?, ?, ?, ?)",
                (username, hashed_password, 0, 0)
            )
            conn.commit()
        except sqlite3.IntegrityError:
            flash("Ce nom d'utilisateur est déjà pris.")
            return redirect(url_for("register"))
        finally:
            conn.close()
        flash("Inscription réussie. Votre compte sera validé par un administrateur.")
        return redirect(url_for("login"))
    return render_template("register.html")

# Route de connexion : vérifie que le compte est validé (sauf si c'est un admin)
@app.route("/login", methods=["GET", "POST"])
def login():
    if request.method == "POST":
        username = request.form["username"]
        password = request.form["password"]
        conn = get_db_connection()
        user = conn.execute("SELECT * FROM users WHERE username = ?", (username,)).fetchone()
        conn.close()
        if user and verify_password(user["password"], password):
            if not user["validated"] and not user["is_admin"]:
                flash("Votre compte n'est pas encore validé par un administrateur.")
                return redirect(url_for("login"))
            session["user_id"] = user["id"]
            session["username"] = user["username"]
            session["is_admin"] = bool(user["is_admin"])
            session["is_superadmin"] = bool(user["is_superadmin"])
            flash("Connexion réussie.")
            return redirect(url_for("dashboard"))
        else:
            flash("Nom d'utilisateur ou mot de passe incorrect.")
            return redirect(url_for("login"))
    return render_template("login.html")

@app.route("/logout")
def logout():
    session.clear()
    flash("Vous êtes déconnecté.")
    return redirect(url_for("login"))

def login_required(func):
    @wraps(func)
    def wrapper(*args, **kwargs):
        if "user_id" not in session:
            flash("Veuillez vous connecter pour accéder à cette page.")
            return redirect(url_for("login"))
        return func(*args, **kwargs)
    return wrapper

@app.route("/")
def home():
    if "user_id" in session:
        return redirect(url_for("dashboard"))
    else:
        return redirect(url_for("login"))

@app.route("/dashboard")
@login_required
def dashboard():
    user_id = session["user_id"]
    conn = get_db_connection()
    works = conn.execute("SELECT * FROM works WHERE user_id = ?", (user_id,)).fetchall()
    conn.close()
    return render_template("dashboard.html", works=works, reading_types=READING_TYPES)


@app.route("/users")
@login_required
def community_directory():
    query = request.args.get("q", "").strip()
    viewer_id = session["user_id"]
    conn = get_db_connection()
    sql = (
        "SELECT id, username, display_name, bio, avatar_path FROM users "
        "WHERE validated = 1 AND is_public = 1 AND id != ?"
    )
    params = [viewer_id]
    if query:
        like_pattern = f"%{query.lower()}%"
        sql += " AND (LOWER(username) LIKE ? OR LOWER(COALESCE(display_name, '')) LIKE ?)"
        params.extend([like_pattern, like_pattern])
    sql += " ORDER BY LOWER(COALESCE(display_name, username))"
    users = [dict(row) for row in conn.execute(sql, params).fetchall()]
    conn.close()
    return render_template("users.html", users=users, query=query)


def _can_view_profile(target_user):
    if target_user is None:
        return False
    if session.get("user_id") == target_user["id"]:
        return True
    if session.get("is_admin"):
        return True
    return bool(target_user["is_public"])


@app.route("/users/<int:user_id>")
@login_required
def user_detail(user_id):
    conn = get_db_connection()
    target_user = conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
    if not target_user:
        conn.close()
        flash("Utilisateur introuvable.")
        return redirect(url_for("community_directory"))

    if not _can_view_profile(target_user):
        conn.close()
        flash("Ce profil est privé.")
        return redirect(url_for("community_directory"))

    works = conn.execute(
        "SELECT * FROM works WHERE user_id = ? ORDER BY LOWER(title)", (user_id,)
    ).fetchall()
    conn.close()
    target = dict(target_user)
    target["is_public"] = bool(target_user["is_public"])
    can_import = session.get("user_id") != target_user["id"]
    return render_template(
        "user_detail.html",
        target_user=target,
        works=works,
        can_import=can_import,
    )


@app.route("/users/<int:user_id>/import/<int:work_id>", methods=["POST"])
@login_required
def import_work(user_id, work_id):
    viewer_id = session["user_id"]
    conn = get_db_connection()
    target_user = conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
    if not target_user:
        conn.close()
        flash("Utilisateur introuvable.")
        return redirect(url_for("community_directory"))

    if not _can_view_profile(target_user):
        conn.close()
        flash("Ce profil est privé.")
        return redirect(url_for("community_directory"))

    if viewer_id == target_user["id"]:
        conn.close()
        flash("Cette œuvre appartient déjà à votre bibliothèque.")
        return redirect(url_for("user_detail", user_id=user_id))

    work = conn.execute(
        "SELECT * FROM works WHERE id = ? AND user_id = ?", (work_id, user_id)
    ).fetchone()
    if not work:
        conn.close()
        flash("Œuvre introuvable.")
        return redirect(url_for("user_detail", user_id=user_id))

    existing = conn.execute(
        """
        SELECT id FROM works
        WHERE user_id = ?
          AND title = ?
          AND COALESCE(link, '') = COALESCE(?, '')
        """,
        (viewer_id, work["title"], work["link"]),
    ).fetchone()
    if existing:
        conn.close()
        flash("Cette œuvre est déjà présente dans votre bibliothèque.")
        return redirect(url_for("user_detail", user_id=user_id))

    reading_type = work["reading_type"] if work["reading_type"] else READING_TYPES[0]

    conn.execute(
        """
        INSERT INTO works (title, chapter, link, status, image_path, reading_type, user_id)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        """,
        (
            work["title"],
            work["chapter"],
            work["link"],
            work["status"],
            work["image_path"],
            reading_type,
            viewer_id,
        ),
    )
    conn.commit()
    conn.close()
    flash("Œuvre ajoutée à votre liste de lecture.")
    return redirect(url_for("user_detail", user_id=user_id))


@app.route("/profile", methods=["GET", "POST"])
@login_required
def profile():
    conn = get_db_connection()
    user = conn.execute("SELECT * FROM users WHERE id = ?", (session["user_id"],)).fetchone()
    if not user:
        conn.close()
        session.clear()
        flash("Votre compte n'est plus disponible. Merci de vous reconnecter.")
        return redirect(url_for("login"))

    user_data = dict(user)
    user_data["is_public"] = bool(user_data.get("is_public", 1))

    if request.method == "POST":
        previous_avatar = user_data.get("avatar_path")
        new_avatar_rel = None
        new_username = request.form.get("username", "").strip()
        display_name = request.form.get("display_name", "").strip()
        email = request.form.get("email", "").strip()
        bio = request.form.get("bio", "").strip()
        current_password = request.form.get("current_password", "")
        new_password = request.form.get("new_password", "")
        confirm_password = request.form.get("confirm_password", "")

        if not new_username:
            flash("Le pseudo ne peut pas être vide.")
            conn.close()
            return redirect(url_for("profile"))

        if len(new_username) > 50:
            flash("Le pseudo ne doit pas dépasser 50 caractères.")
            conn.close()
            return redirect(url_for("profile"))

        updates = {}
        requires_password_check = False

        if new_username != user_data.get("username"):
            requires_password_check = True
            updates["username"] = new_username

        if new_password or confirm_password:
            requires_password_check = True
            if new_password != confirm_password:
                flash("Les nouveaux mots de passe ne correspondent pas.")
                conn.close()
                return redirect(url_for("profile"))
            if len(new_password) < 8:
                flash("Le nouveau mot de passe doit contenir au moins 8 caractères.")
                conn.close()
                return redirect(url_for("profile"))
            updates["password"] = generate_password_hash(new_password, method="pbkdf2:sha256")

        if requires_password_check:
            if not current_password:
                flash("Veuillez renseigner votre mot de passe actuel pour confirmer ces modifications.")
                conn.close()
                return redirect(url_for("profile"))
            if not verify_password(user_data["password"], current_password):
                flash("Mot de passe actuel incorrect.")
                conn.close()
                return redirect(url_for("profile"))

        if len(display_name) > 80:
            flash("Le nom affiché ne doit pas dépasser 80 caractères.")
            conn.close()
            return redirect(url_for("profile"))
        if display_name != (user_data.get("display_name") or ""):
            updates["display_name"] = display_name or None

        if email != (user_data.get("email") or ""):
            if email and "@" not in email:
                flash("Veuillez saisir une adresse e-mail valide.")
                conn.close()
                return redirect(url_for("profile"))
            if email and len(email) > 120:
                flash("L'adresse e-mail est trop longue.")
                conn.close()
                return redirect(url_for("profile"))
            updates["email"] = email or None

        if len(bio) > 500:
            flash("La biographie est limitée à 500 caractères.")
            conn.close()
            return redirect(url_for("profile"))
        if bio != (user_data.get("bio") or ""):
            updates["bio"] = bio or None

        visibility = request.form.get("is_public")
        if visibility is not None:
            desired_public = 1 if visibility == "1" else 0
            if desired_public != int(user_data.get("is_public", True)):
                updates["is_public"] = desired_public

        avatar = request.files.get("avatar")
        if avatar and avatar.filename:
            if not allowed_file(avatar.filename):
                flash("Format d'image non supporté. Formats acceptés : png, jpg, jpeg, gif.")
                conn.close()
                return redirect(url_for("profile"))
            safe_name = secure_filename(avatar.filename)
            if not safe_name:
                safe_name = "avatar"
            avatar_filename = f"{uuid.uuid4().hex}_{safe_name}"
            avatar_folder = app.config["PROFILE_UPLOAD_FOLDER"]
            if not os.path.isabs(avatar_folder):
                avatar_folder = os.path.join(app.root_path, avatar_folder)
            os.makedirs(avatar_folder, exist_ok=True)
            avatar_full_path = os.path.join(avatar_folder, avatar_filename)
            avatar.save(avatar_full_path)
            new_avatar_rel = f"avatars/{avatar_filename}"
            updates["avatar_path"] = new_avatar_rel

        if updates:
            placeholders = ", ".join(f"{field} = ?" for field in updates.keys())
            values = list(updates.values()) + [session["user_id"]]
            try:
                conn.execute(f"UPDATE users SET {placeholders} WHERE id = ?", values)
                conn.commit()
            except sqlite3.IntegrityError:
                conn.rollback()
                if new_avatar_rel:
                    _delete_media_file(new_avatar_rel, "PROFILE_UPLOAD_FOLDER")
                flash("Ce pseudo est déjà utilisé.")
                conn.close()
                return redirect(url_for("profile"))

            if "username" in updates:
                session["username"] = updates["username"]

            if new_avatar_rel and previous_avatar and previous_avatar != new_avatar_rel:
                remaining = conn.execute(
                    "SELECT COUNT(*) FROM users WHERE avatar_path = ? AND id != ?",
                    (previous_avatar, session["user_id"]),
                ).fetchone()[0]
                if remaining == 0:
                    _delete_media_file(previous_avatar, "PROFILE_UPLOAD_FOLDER")

            flash("Profil mis à jour.")
        else:
            flash("Aucune modification à enregistrer.")
        conn.close()
        return redirect(url_for("profile"))

    conn.close()
    return render_template("profile.html", user=user_data)


@app.route("/add_work", methods=["GET", "POST"])
@login_required
def add_work():
    if request.method == "POST":
        title = request.form["title"]
        if len(title) > 30:
            flash("Le titre ne doit pas dépasser 30 caractères.")
            return redirect(url_for("add_work"))
        link = request.form["link"]
        status = request.form["status"]
        chapter = request.form.get("chapter", 0)
        chapter = int(chapter) if chapter else 0
        reading_type = request.form.get("reading_type", "").strip()
        if not reading_type:
            reading_type = READING_TYPES[0]
        elif reading_type not in READING_TYPES:
            flash("Type de lecture invalide.")
            return redirect(url_for("add_work"))

        image_path = None
        if "image" in request.files:
            file = request.files["image"]
            if file and allowed_file(file.filename):
                safe_name = secure_filename(file.filename)
                if not safe_name:
                    safe_name = "work"
                filename = f"{uuid.uuid4().hex}_{safe_name}"
                storage_dir = app.config["UPLOAD_FOLDER"]
                if not os.path.isabs(storage_dir):
                    storage_dir = os.path.join(app.root_path, storage_dir)
                os.makedirs(storage_dir, exist_ok=True)
                filepath = os.path.join(storage_dir, filename)
                file.save(filepath)
                image_path = f"images/{filename}"

        conn = get_db_connection()
        conn.execute("""
            INSERT INTO works (title, chapter, link, status, image_path, reading_type, user_id)
            VALUES (?, ?, ?, ?, ?, ?, ?)
        """, (title, chapter, link, status, image_path, reading_type, session["user_id"]))
        conn.commit()
        conn.close()

        flash("Oeuvre ajoutée avec succès !")
        return redirect(url_for("dashboard"))
    return render_template("add_work.html", reading_types=READING_TYPES)

# Endpoints API pour incrémenter/décrémenter les chapitres (via AJAX)
@app.route("/api/increment/<int:work_id>", methods=["POST"])
@login_required
def api_increment(work_id):
    user_id = session["user_id"]
    conn = get_db_connection()
    conn.execute("UPDATE works SET chapter = chapter + 1 WHERE id = ? AND user_id = ?", (work_id, user_id))
    conn.commit()
    new_chapter = conn.execute("SELECT chapter FROM works WHERE id = ? AND user_id = ?", (work_id, user_id)).fetchone()["chapter"]
    conn.close()
    return jsonify({"success": True, "chapter": new_chapter})

@app.route("/api/decrement/<int:work_id>", methods=["POST"])
@login_required
def api_decrement(work_id):
    user_id = session["user_id"]
    conn = get_db_connection()
    conn.execute("UPDATE works SET chapter = CASE WHEN chapter > 0 THEN chapter - 1 ELSE 0 END WHERE id = ? AND user_id = ?", (work_id, user_id))
    conn.commit()
    new_chapter = conn.execute("SELECT chapter FROM works WHERE id = ? AND user_id = ?", (work_id, user_id)).fetchone()["chapter"]
    conn.close()
    return jsonify({"success": True, "chapter": new_chapter})

@app.route("/delete/<int:work_id>")
@login_required
def delete(work_id):
    user_id = session["user_id"]
    conn = get_db_connection()
    work = conn.execute(
        "SELECT image_path FROM works WHERE id = ? AND user_id = ?",
        (work_id, user_id),
    ).fetchone()

    image_path = work["image_path"] if work else None
    remaining_references = 0
    if image_path:
        remaining_references = conn.execute(
            "SELECT COUNT(*) FROM works WHERE image_path = ? AND id != ?",
            (image_path, work_id),
        ).fetchone()[0]

    conn.execute("DELETE FROM works WHERE id = ? AND user_id = ?", (work_id, user_id))
    conn.commit()
    conn.close()

    if image_path and remaining_references == 0:
        _delete_media_file(image_path, "UPLOAD_FOLDER")
    flash("Oeuvre supprimée.")
    return redirect(url_for("dashboard"))

# Routes d'administration

# Page de gestion des comptes
@app.route("/admin/accounts")
@admin_required
def admin_accounts():
    conn = get_db_connection()
    users = conn.execute("SELECT * FROM users").fetchall()
    conn.close()
    return render_template("admin_accounts.html", users=users)

# Approve un compte utilisateur (validation)
@app.route("/admin/approve/<int:user_id>")
@admin_required
def approve_account(user_id):
    conn = get_db_connection()
    conn.execute("UPDATE users SET validated = 1 WHERE id = ?", (user_id,))
    conn.commit()
    conn.close()
    flash("Compte approuvé.")
    return redirect(url_for("admin_accounts"))



@app.route("/admin/delete_account/<int:user_id>")
@admin_required
def delete_account(user_id):
    conn = get_db_connection()
    target_user = conn.execute("SELECT * FROM users WHERE id = ?", (user_id,)).fetchone()
    if not target_user:
        flash("Compte introuvable.")
        conn.close()
        return redirect(url_for("admin_accounts"))
    
    # Si le compte ciblé est administrateur (ou super‑admin)
    if target_user["is_admin"]:
        # Seul un super‑admin peut supprimer un compte admin
        if not session.get("is_superadmin"):
            flash("Seul un super‑admin peut supprimer un compte administrateur.")
            conn.close()
            return redirect(url_for("admin_accounts"))
        # Interdire la suppression d'un compte super‑admin
        if target_user["is_superadmin"]:
            flash("Impossible de supprimer un compte super‑admin.")
            conn.close()
            return redirect(url_for("admin_accounts"))
    
    conn.execute("DELETE FROM users WHERE id = ?", (user_id,))
    conn.commit()
    conn.close()
    flash("Compte supprimé.")
    return redirect(url_for("admin_accounts"))

@app.route("/admin/promote/<int:user_id>")
@admin_required
def promote_account(user_id):
    conn = get_db_connection()
    conn.execute("UPDATE users SET is_admin = 1, validated = 1 WHERE id = ?", (user_id,))
    conn.commit()
    conn.close()
    flash("Le compte a été promu en administrateur.")
    return redirect(url_for("admin_accounts"))


if __name__ == "__main__":
    app.run(debug=True)
