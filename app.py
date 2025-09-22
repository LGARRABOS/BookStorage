import os
import sqlite3
from functools import wraps
from flask import Flask, render_template, request, redirect, url_for, session, flash, jsonify
from werkzeug.security import generate_password_hash, check_password_hash
from werkzeug.utils import secure_filename

app = Flask(__name__)
app.secret_key = "votre_cle_secrete"  # Remplacez par une clé plus sûre

DATABASE = "database.db"
UPLOAD_FOLDER = os.path.join("static", "images")
app.config["UPLOAD_FOLDER"] = UPLOAD_FOLDER
ALLOWED_EXTENSIONS = {"png", "jpg", "jpeg", "gif"}

def allowed_file(filename):
    return '.' in filename and filename.rsplit('.', 1)[1].lower() in ALLOWED_EXTENSIONS

def get_db_connection():
    conn = sqlite3.connect(DATABASE)
    conn.row_factory = sqlite3.Row
    return conn

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
        hashed_password = generate_password_hash(password)
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
        if user and check_password_hash(user["password"], password):
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
    return render_template("dashboard.html", works=works)

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

        image_path = None
        if "image" in request.files:
            file = request.files["image"]
            if file and allowed_file(file.filename):
                filename = secure_filename(file.filename)
                filepath = os.path.join(app.config["UPLOAD_FOLDER"], filename)
                file.save(filepath)
                image_path = f"images/{filename}"

        conn = get_db_connection()
        conn.execute("""
            INSERT INTO works (title, chapter, link, status, image_path, user_id)
            VALUES (?, ?, ?, ?, ?, ?)
        """, (title, chapter, link, status, image_path, session["user_id"]))
        conn.commit()
        conn.close()

        flash("Oeuvre ajoutée avec succès !")
        return redirect(url_for("dashboard"))
    return render_template("add_work.html")

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
    conn.execute("DELETE FROM works WHERE id = ? AND user_id = ?", (work_id, user_id))
    conn.commit()
    conn.close()
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
