// mu-app runtime — provides standard layouts, components, and lifecycle
// Apps declare config + data functions, the runtime handles everything else
(function() {
  'use strict';

  // --- Component library ---

  function esc(s) {
    var d = document.createElement('div');
    d.textContent = s == null ? '' : String(s);
    return d.innerHTML;
  }

  function renderCard(item) {
    var html = '<div class="mu-card">';
    if (item.icon) html += '<div class="mu-card-icon">' + esc(item.icon) + '</div>';
    html += '<div class="mu-card-body">';
    if (item.title) html += '<div class="mu-card-title">' + esc(item.title) + '</div>';
    if (item.subtitle) html += '<div class="mu-card-subtitle">' + esc(item.subtitle) + '</div>';
    if (item.description) html += '<div class="mu-card-desc">' + esc(item.description) + '</div>';
    if (item.html) html += '<div class="mu-card-custom">' + item.html + '</div>';
    html += '</div>';
    if (item.value != null) {
      html += '<div class="mu-card-right">';
      html += '<div class="mu-card-value">' + esc(item.value) + '</div>';
      if (item.badge != null) {
        var color = item.badgeColor || 'gray';
        html += '<div class="mu-badge mu-badge-' + color + '">' + esc(item.badge) + '</div>';
      }
      html += '</div>';
    }
    html += '</div>';
    return html;
  }

  function renderStat(item) {
    var html = '<div class="mu-stat">';
    html += '<div class="mu-stat-value">' + esc(item.value) + '</div>';
    html += '<div class="mu-stat-label">' + esc(item.label) + '</div>';
    if (item.change != null) {
      var cls = parseFloat(item.change) >= 0 ? 'mu-up' : 'mu-down';
      html += '<div class="mu-stat-change ' + cls + '">' + esc(item.change) + '</div>';
    }
    html += '</div>';
    return html;
  }

  function renderTable(data) {
    if (!data || !data.columns || !data.rows) return '';
    var html = '<table class="mu-table"><thead><tr>';
    data.columns.forEach(function(col) {
      html += '<th>' + esc(typeof col === 'string' ? col : col.label || col.key) + '</th>';
    });
    html += '</tr></thead><tbody>';
    data.rows.forEach(function(row) {
      html += '<tr>';
      data.columns.forEach(function(col) {
        var key = typeof col === 'string' ? col : col.key;
        var val = row[key];
        html += '<td>' + esc(val == null ? '' : val) + '</td>';
      });
      html += '</tr>';
    });
    html += '</tbody></table>';
    return html;
  }

  // --- Layouts ---

  function renderLayout(layout, sections) {
    var html = '';
    switch (layout) {
      case 'grid':
        html += '<div class="mu-grid">';
        sections.forEach(function(s) { html += '<div class="mu-grid-cell">' + renderSection(s) + '</div>'; });
        html += '</div>';
        break;
      case 'dashboard':
        var stats = sections.filter(function(s) { return s.type === 'stats'; });
        var rest = sections.filter(function(s) { return s.type !== 'stats'; });
        if (stats.length) html += renderSection(stats[0]);
        html += '<div class="mu-grid">';
        rest.forEach(function(s) { html += '<div class="mu-grid-cell">' + renderSection(s) + '</div>'; });
        html += '</div>';
        break;
      default: // list, single column
        sections.forEach(function(s) { html += renderSection(s); });
    }
    return html;
  }

  function renderSection(section) {
    var html = '<div class="mu-section" id="section-' + (section.id || '') + '">';
    if (section.title) html += '<h3 class="mu-section-title">' + esc(section.title) + '</h3>';
    html += '<div class="mu-section-content">';

    if (section.loading) {
      html += '<div class="mu-loading">Loading...</div>';
    } else if (section.error) {
      html += '<div class="mu-error">' + esc(section.error) + '</div>';
    } else if (section.type === 'stats' && section.items) {
      html += '<div class="mu-stats-row">';
      section.items.forEach(function(item) { html += renderStat(item); });
      html += '</div>';
    } else if (section.type === 'table' && section.data) {
      html += renderTable(section.data);
    } else if (section.type === 'html' && section.html) {
      html += section.html;
    } else if (section.items) {
      section.items.forEach(function(item) { html += renderCard(item); });
    }

    html += '</div></div>';
    return html;
  }

  // --- Runtime ---

  var appConfig = null;
  var appSections = {};

  function render() {
    var root = document.getElementById('mu-app');
    if (!root) return;
    var sections = Object.keys(appSections).map(function(k) { return appSections[k]; });
    // Sort by order if specified
    sections.sort(function(a, b) { return (a.order || 0) - (b.order || 0); });
    root.innerHTML = renderLayout(appConfig.layout || 'list', sections);
  }

  // Public API for apps
  window.app = {
    // Configure the app
    config: function(cfg) {
      appConfig = cfg;
      // Set page title
      if (cfg.name) document.title = cfg.name;
      // Render header
      var header = document.getElementById('mu-app-header');
      if (header && cfg.name) header.textContent = cfg.name;
      // Set up tabs if specified
      if (cfg.tabs) {
        var tabBar = document.getElementById('mu-app-tabs');
        if (tabBar) {
          var html = '';
          cfg.tabs.forEach(function(tab, i) {
            html += '<button class="mu-tab' + (i === 0 ? ' active' : '') + '" onclick="app.switchTab(\'' + esc(tab.id) + '\')">' + esc(tab.label) + '</button>';
          });
          tabBar.innerHTML = html;
        }
      }
      // Set up search if specified
      if (cfg.search) {
        var searchBar = document.getElementById('mu-app-search');
        if (searchBar) searchBar.style.display = 'flex';
      }
    },

    // Define a section with loading state, then load data
    section: function(id, opts) {
      appSections[id] = {
        id: id,
        title: opts.title || '',
        type: opts.type || 'list',
        loading: true,
        order: opts.order || 0,
      };
      render();

      if (opts.load) {
        Promise.resolve(opts.load()).then(function(result) {
          appSections[id].loading = false;
          if (result.items) appSections[id].items = result.items;
          if (result.data) appSections[id].data = result.data;
          if (result.html) { appSections[id].html = result.html; appSections[id].type = 'html'; }
          if (result.error) appSections[id].error = result.error;
          render();
        }).catch(function(err) {
          appSections[id].loading = false;
          appSections[id].error = err.message || 'Failed to load';
          render();
        });
      }
    },

    // Update a section
    update: function(id, data) {
      if (!appSections[id]) return;
      Object.keys(data).forEach(function(k) { appSections[id][k] = data[k]; });
      appSections[id].loading = false;
      render();
    },

    // Switch tab
    switchTab: function(tabId) {
      document.querySelectorAll('.mu-tab').forEach(function(el) { el.className = 'mu-tab'; });
      event.target.className = 'mu-tab active';
      if (appConfig && appConfig.onTab) appConfig.onTab(tabId);
    },

    // Handle search
    onSearch: null,
  };

  // Search handler
  document.addEventListener('DOMContentLoaded', function() {
    var searchInput = document.getElementById('mu-search-input');
    if (searchInput) {
      searchInput.addEventListener('keydown', function(e) {
        if (e.key === 'Enter' && app.onSearch) {
          app.onSearch(searchInput.value.trim());
        }
      });
    }
  });

})();
