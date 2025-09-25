(function () {
  const titleInput = document.getElementById("title");
  const linkInput = document.getElementById("link");
  const readingTypeSelect = document.getElementById("reading_type");
  const coverField = document.getElementById("metadata_cover_url");
  const infoField = document.getElementById("metadata_info_url");
  const searchButton = document.getElementById("metadata-search");
  const resultsSection = document.getElementById("metadata-results");
  const resultsList = document.getElementById("metadata-results-list");
  const statusLabel = document.getElementById("metadata-status");

  if (!titleInput || !searchButton || !resultsSection || !resultsList || !statusLabel) {
    return;
  }

  function setStatus(message, isError) {
    statusLabel.textContent = message;
    if (isError) {
      statusLabel.classList.add("error");
    } else {
      statusLabel.classList.remove("error");
    }
  }

  function clearResults() {
    resultsList.innerHTML = "";
  }

  function applySuggestion(suggestion) {
    titleInput.value = suggestion.title || titleInput.value;
    if (suggestion.info_url) {
      infoField.value = suggestion.info_url;
      if (linkInput) {
        linkInput.value = suggestion.info_url;
      }
    }
    if (readingTypeSelect && suggestion.reading_type) {
      const option = Array.from(readingTypeSelect.options).find(
        (opt) => opt.value === suggestion.reading_type
      );
      if (option) {
        readingTypeSelect.value = option.value;
      }
    }
    if (suggestion.cover_url) {
      coverField.value = suggestion.cover_url;
    } else {
      coverField.value = "";
    }
    setStatus("Suggestion appliquée. Vous pouvez ajuster les informations avant l'enregistrement.", false);
  }

  function createResultCard(suggestion) {
    const card = document.createElement("article");
    card.className = "metadata-card";
    card.setAttribute("role", "listitem");

    const header = document.createElement("header");
    if (suggestion.cover_url) {
      const image = document.createElement("img");
      image.src = suggestion.cover_url;
      image.alt = `Couverture proposée pour ${suggestion.title}`;
      image.className = "metadata-cover";
      header.appendChild(image);
    }

    const info = document.createElement("div");
    info.className = "metadata-info";

    const title = document.createElement("h3");
    title.textContent = suggestion.title;
    info.appendChild(title);

    if (suggestion.authors && suggestion.authors.length > 0) {
      const authors = document.createElement("p");
      authors.textContent = `Auteur(s) : ${suggestion.authors.join(", ")}`;
      info.appendChild(authors);
    }

    if (suggestion.published_year) {
      const year = document.createElement("p");
      year.textContent = `Première publication : ${suggestion.published_year}`;
      info.appendChild(year);
    }

    if (suggestion.summary) {
      const summary = document.createElement("p");
      summary.textContent = suggestion.summary;
      info.appendChild(summary);
    }

    header.appendChild(info);
    card.appendChild(header);

    const footer = document.createElement("div");
    footer.className = "metadata-footer";

    const typeBadge = document.createElement("span");
    typeBadge.className = "badge";
    typeBadge.textContent = suggestion.reading_type || "Type";
    footer.appendChild(typeBadge);

    const useButton = document.createElement("button");
    useButton.type = "button";
    useButton.className = "btn btn-primary";
    useButton.textContent = "Utiliser";
    useButton.addEventListener("click", function () {
      applySuggestion(suggestion);
    });

    footer.appendChild(useButton);
    card.appendChild(footer);

    return card;
  }

  function renderResults(results) {
    clearResults();
    if (!results || results.length === 0) {
      resultsSection.hidden = false;
      setStatus("Aucune proposition trouvée. Essayez un autre titre ou ajoutez l'œuvre manuellement.", true);
      return;
    }

    results.forEach((suggestion) => {
      resultsList.appendChild(createResultCard(suggestion));
    });
    resultsSection.hidden = false;
    setStatus("Sélectionnez l'une des suggestions pour préremplir le formulaire.", false);
  }

  async function triggerSearch() {
    const query = titleInput.value.trim();
    if (!query) {
      setStatus("Indiquez d'abord un titre ou un identifiant.", true);
      resultsSection.hidden = true;
      clearResults();
      return;
    }

    searchButton.disabled = true;
    setStatus("Recherche en cours…", false);
    try {
      const response = await fetch(`/api/metadata/search?q=${encodeURIComponent(query)}`);
      if (!response.ok) {
        throw new Error(`Statut ${response.status}`);
      }
      const payload = await response.json();
      renderResults(payload.results || []);
    } catch (error) {
      console.error("metadata lookup failed", error);
      setStatus("La recherche a échoué. Vérifiez votre connexion et réessayez.", true);
      resultsSection.hidden = false;
      clearResults();
    } finally {
      searchButton.disabled = false;
    }
  }

  searchButton.addEventListener("click", triggerSearch);
})();
