package mu.micro.launcher

import android.annotation.SuppressLint
import android.app.Activity
import android.app.AlertDialog
import android.app.role.RoleManager
import android.content.ActivityNotFoundException
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Message
import android.provider.Settings
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.webkit.CookieManager
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.ListView
import android.widget.ProgressBar
import android.widget.TextView
import android.widget.Toast

/** Native home/app navigation only. Micro owns all assistant UI and account state. */
class LauncherActivity : Activity() {
    private lateinit var web: WebView
    private lateinit var content: FrameLayout
    private lateinit var error: LinearLayout
    private lateinit var progress: ProgressBar
    private lateinit var drawer: ListView
    private var failed = false
    private var fileCallback: ValueCallback<Array<Uri>>? = null
    private var drawerOpen = false

    override fun onCreate(state: Bundle?) {
        super.onCreate(state)
        if (Build.VERSION.SDK_INT >= 30) window.setDecorFitsSystemWindows(false)
        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setBackgroundColor(Color.WHITE)
            setOnApplyWindowInsetsListener { view, insets ->
                // Includes the keyboard on older Android; avoids covering the composer.
                view.setPadding(insets.systemWindowInsetLeft, insets.systemWindowInsetTop,
                    insets.systemWindowInsetRight, insets.systemWindowInsetBottom)
                insets
            }
        }
        content = FrameLayout(this)
        web = WebView(this)
        configureWebView()
        content.addView(web, FrameLayout.LayoutParams(-1, -1))
        error = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER
            setPadding(dp(24), dp(24), dp(24), dp(24))
            setBackgroundColor(Color.WHITE)
            addView(TextView(context).apply {
                setText(R.string.load_error)
                textSize = 18f
                gravity = Gravity.CENTER
            })
            addView(button("Retry") { web.reload() })
            visibility = View.GONE
        }
        content.addView(error, FrameLayout.LayoutParams(-1, -1))
        drawer = ListView(this).apply { setBackgroundColor(Color.WHITE); visibility = View.GONE }
        content.addView(drawer, FrameLayout.LayoutParams(-1, -1))
        progress = ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal).apply { max = 100 }
        root.addView(progress, LinearLayout.LayoutParams(-1, dp(2)))
        root.addView(content, LinearLayout.LayoutParams(-1, 0, 1f))
        val toolbar = LinearLayout(this).apply {
            gravity = Gravity.CENTER
            setPadding(dp(4), 0, dp(4), 0)
        }
        toolbar.addView(button("Home") { home() }, LinearLayout.LayoutParams(0, dp(48), 1f))
        toolbar.addView(button("Apps") { showApps() }, LinearLayout.LayoutParams(0, dp(48), 1f))
        toolbar.addView(button("Settings") { showSettings() }, LinearLayout.LayoutParams(0, dp(48), 1f))
        root.addView(toolbar)
        setContentView(root)
        // Reload Home after process death; cookies persist, but private form/history
        // state is never serialized into an Activity bundle or Android backup.
        web.loadUrl(Navigation.HOME)
    }

    private fun dp(n: Int) = (n * resources.displayMetrics.density).toInt()
    private fun button(label: String, action: () -> Unit) = Button(this).apply {
        text = label
        isAllCaps = false
        setTextColor(Color.rgb(45, 45, 45))
        setBackgroundColor(Color.WHITE)
        setOnClickListener { action() }
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun configureWebView() {
        web.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true
            allowFileAccess = false
            allowContentAccess = false
            mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
            setSupportMultipleWindows(true)
            javaScriptCanOpenWindowsAutomatically = false
        }
        CookieManager.getInstance().apply {
            setAcceptCookie(true)
            setAcceptThirdPartyCookies(web, false)
        }
        web.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                if (!request.isForMainFrame) return false
                return route(request.url.toString(), request.hasGesture())
            }
            override fun onPageStarted(view: WebView, url: String, icon: Bitmap?) {
                failed = false
                error.visibility = View.GONE
                progress.visibility = View.VISIBLE
            }
            override fun onPageFinished(view: WebView, url: String) {
                progress.visibility = View.GONE
                CookieManager.getInstance().flush()
                if (failed) error.visibility = View.VISIBLE
            }
            override fun onReceivedError(view: WebView, request: WebResourceRequest, problem: WebResourceError) {
                if (request.isForMainFrame) showError()
            }
            override fun onReceivedHttpError(view: WebView, request: WebResourceRequest, response: WebResourceResponse) {
                if (request.isForMainFrame && response.statusCode >= 500) showError()
            }
            // Default SSL handling cancels bad certificates. Never bypass it.
        }
        web.webChromeClient = object : WebChromeClient() {
            override fun onProgressChanged(view: WebView, value: Int) { progress.progress = value }
            override fun onCreateWindow(view: WebView, dialog: Boolean, gesture: Boolean, result: Message): Boolean {
                if (!gesture) return false
                val popup = WebView(this@LauncherActivity)
                popup.webViewClient = object : WebViewClient() {
                    override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                        val url = request.url.toString()
                        if (!route(url, true)) web.loadUrl(url)
                        popup.destroy()
                        return true
                    }
                }
                (result.obj as WebView.WebViewTransport).webView = popup
                result.sendToTarget()
                return true
            }
            override fun onShowFileChooser(view: WebView, callback: ValueCallback<Array<Uri>>, params: FileChooserParams): Boolean {
                fileCallback?.onReceiveValue(null)
                fileCallback = callback
                try { startActivityForResult(params.createIntent(), FILE_PICKER) }
                catch (_: ActivityNotFoundException) { fileCallback?.onReceiveValue(null); fileCallback = null }
                return true
            }
        }
        web.setDownloadListener { url, _, _, _, _ -> openExternal(url) }
    }

    private fun route(url: String, userGesture: Boolean): Boolean {
        return when (Navigation.destination(url)) {
            Navigation.Destination.MICRO -> false
            Navigation.Destination.BROWSER_LOGIN -> {
                AlertDialog.Builder(this).setTitle("Sign in to Micro")
                    .setMessage("Use your Micro username and password here. If you normally use Google, open Account in your browser and set a password first. Browser sign-in is separate from this launcher.")
                    .setPositiveButton("Open account") { _, _ -> openExternal("https://micro.mu/account") }
                    .setNegativeButton("Cancel", null).show()
                true
            }
            Navigation.Destination.EXTERNAL -> {
                // Do not let redirects silently dial, compose mail, or launch apps.
                if (userGesture) openExternal(url)
                else AlertDialog.Builder(this).setMessage("Open this link outside Micro?")
                    .setPositiveButton("Open") { _, _ -> openExternal(url) }
                    .setNegativeButton("Cancel", null).show()
                true
            }
            Navigation.Destination.BLOCKED -> true
        }
    }

    private fun openExternal(url: String) {
        val destination = Navigation.destination(url)
        if (destination == Navigation.Destination.BLOCKED) return
        val uri = Uri.parse(url)
        val action = if (uri.scheme == "tel") Intent.ACTION_DIAL else Intent.ACTION_VIEW
        launch(Intent(action, uri).addCategory(Intent.CATEGORY_BROWSABLE))
    }

    private fun launch(intent: Intent) {
        try { startActivity(intent) }
        catch (_: ActivityNotFoundException) { Toast.makeText(this, "No app available to open this", Toast.LENGTH_SHORT).show() }
        catch (_: SecurityException) { Toast.makeText(this, "This app could not be opened", Toast.LENGTH_SHORT).show() }
    }

    private fun showError() {
        failed = true
        error.visibility = View.VISIBLE
        progress.visibility = View.GONE
    }

    private fun closeDrawer() { drawerOpen = false; drawer.visibility = View.GONE }
    private fun home() { closeDrawer(); web.loadUrl(Navigation.HOME) }

    private fun showApps() {
        val query = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
        val apps = packageManager.queryIntentActivities(query, 0)
            .filter { it.activityInfo.packageName != packageName }
            .sortedBy { it.loadLabel(packageManager).toString().lowercase() }
        drawer.adapter = ArrayAdapter(this, android.R.layout.simple_list_item_1,
            apps.map { it.loadLabel(packageManager).toString() })
        drawer.setOnItemClickListener { _, _, position, _ ->
            val info = apps[position].activityInfo
            closeDrawer()
            launch(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
                .setClassName(info.packageName, info.name)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_RESET_TASK_IF_NEEDED))
        }
        drawerOpen = true
        drawer.visibility = View.VISIBLE
    }

    private fun showSettings() {
        AlertDialog.Builder(this).setTitle("Micro")
            .setItems(arrayOf("Set default home app", "Android settings", "Sign-in help", "Open Micro in browser")) { _, which ->
                when (which) {
                    0 -> chooseHome()
                    1 -> launch(Intent(Settings.ACTION_SETTINGS))
                    2 -> route("https://micro.mu/oauth2/google", true)
                    3 -> openExternal(Navigation.HOME)
                }
            }.show()
    }

    private fun chooseHome() {
        if (Build.VERSION.SDK_INT >= 29) {
            val roles = getSystemService(RoleManager::class.java)
            if (roles.isRoleAvailable(RoleManager.ROLE_HOME) && !roles.isRoleHeld(RoleManager.ROLE_HOME)) {
                startActivityForResult(roles.createRequestRoleIntent(RoleManager.ROLE_HOME), HOME_ROLE)
                return
            }
        }
        launch(Intent(Settings.ACTION_HOME_SETTINGS))
    }

    override fun onNewIntent(intent: Intent) { super.onNewIntent(intent); setIntent(intent); home() }
    @Deprecated("Legacy Back supports Android 8 through current Android")
    override fun onBackPressed() {
        if (drawerOpen) closeDrawer()
        else if (web.canGoBack()) web.goBack()
        // At the root, stay Home instead of exiting/relaunching the default launcher.
    }
    @Deprecated("Framework file picker result")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == FILE_PICKER) {
            fileCallback?.onReceiveValue(WebChromeClient.FileChooserParams.parseResult(resultCode, data))
            fileCallback = null
        }
    }
    override fun onPause() { CookieManager.getInstance().flush(); web.onPause(); super.onPause() }
    override fun onResume() { super.onResume(); if (::web.isInitialized) web.onResume() }
    override fun onDestroy() {
        fileCallback?.onReceiveValue(null)
        (web.parent as? ViewGroup)?.removeView(web)
        web.destroy()
        super.onDestroy()
    }
    companion object { private const val FILE_PICKER = 1; private const val HOME_ROLE = 2 }
}
