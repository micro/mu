package mu.micro.launcher

import android.app.Activity
import android.app.AlertDialog
import android.content.Intent
import android.graphics.Color
import android.os.Bundle
import android.text.InputType
import android.view.Gravity
import android.view.View
import android.view.WindowManager
import android.view.inputmethod.EditorInfo
import android.widget.*
import mobile.Device
import mobile.Mobile
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.util.concurrent.Executors

/** A native conversation with Mu running in this process, independent of the website. */
class LocalAssistantActivity : Activity() {
    private val worker = Executors.newSingleThreadExecutor()
    private var runtime: mobile.Runtime? = null
    private var key = "" // Deliberately never persisted or placed in savedInstanceState.
    private var busy = false
    private lateinit var transcript: LinearLayout
    private lateinit var scroll: ScrollView
    private lateinit var input: EditText
    private lateinit var send: Button
    private lateinit var status: TextView
    private lateinit var settings: Button
    private val prefs by lazy { getSharedPreferences("local-assistant", MODE_PRIVATE) }
    private fun dp(n: Int) = (n * resources.displayMetrics.density).toInt()
    private fun button(label: String, action: () -> Unit) = Button(this).apply {
        text = label; isAllCaps = false
        setTextColor(Color.rgb(45,45,45)); setBackgroundColor(Color.WHITE)
        setOnClickListener { action() }
    }
    override fun onCreate(state: Bundle?) {
        super.onCreate(state)
        window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        if (android.os.Build.VERSION.SDK_INT >= 30) window.setDecorFitsSystemWindows(false)
        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL; setBackgroundColor(Color.WHITE)
            setOnApplyWindowInsetsListener { view, insets ->
                view.setPadding(insets.systemWindowInsetLeft + dp(16), insets.systemWindowInsetTop,
                    insets.systemWindowInsetRight + dp(16), insets.systemWindowInsetBottom)
                insets
            }
        }
        val header = LinearLayout(this).apply { gravity = Gravity.CENTER_VERTICAL }
        header.addView(button("Back") { finish() })
        header.addView(TextView(this).apply { text = "On-device assistant"; textSize = 18f }, LinearLayout.LayoutParams(0,-2,1f))
        settings = button("Model") { configure() }; header.addView(settings)
        root.addView(header)
        status = TextView(this).apply { text = "Loading conversation…"; textSize = 13f; setPadding(0,dp(8),0,dp(8)) }
        root.addView(status)
        transcript = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
        scroll = ScrollView(this).apply { addView(transcript) }
        root.addView(scroll, LinearLayout.LayoutParams(-1,0,1f))
        val composer = LinearLayout(this).apply { gravity = Gravity.CENTER_VERTICAL }
        input = EditText(this).apply {
            hint = "Ask Micro"; maxLines = 4; isSaveEnabled = false
            inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_FLAG_CAP_SENTENCES
            imeOptions = EditorInfo.IME_ACTION_SEND
            setOnEditorActionListener { _, action, _ -> if (action == EditorInfo.IME_ACTION_SEND) { submit(); true } else false }
        }
        composer.addView(input,LinearLayout.LayoutParams(0,-2,1f))
        send = button("Send") { submit() }; send.isEnabled = false
        composer.addView(send); root.addView(composer); setContentView(root)
        val app = applicationContext
        worker.execute {
            try {
                val device = object : Device {
                    override fun apps(): String {
                        val pm = app.packageManager
                        val apps = pm.queryIntentActivities(Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER),0)
                            .filter { it.activityInfo.packageName != app.packageName }
                            .distinctBy { it.activityInfo.packageName }
                            .sortedBy { it.loadLabel(pm).toString().lowercase() }
                        val out = JSONArray()
                        apps.forEach { out.put(JSONObject().put("name",it.loadLabel(pm).toString()).put("package",it.activityInfo.packageName)) }
                        return out.toString()
                    }
                }
                synchronized(LocalAssistantActivity::class.java) {
                    if (!transportReady) { Mobile.configureTransport(); transportReady = true }
                    if (sharedRuntime == null) sharedRuntime = Mobile.open(File(app.filesDir,"assistant").absolutePath,device)
                    runtime = sharedRuntime
                }
                val history = JSONArray(runtime!!.history().let { if(it=="null") "[]" else it })
                runOnUiThread {
                    if (!isFinishing && !isDestroyed) {
                        for (i in 0 until history.length()) {
                            val turn=history.getJSONObject(i); message("You",turn.getString("q")); message("Micro",turn.getString("a"))
                        }
                        status.text = "Saved on this phone. Set a model to chat."; send.isEnabled = true
                    }
                }
            } catch (_: Exception) { runOnUiThread { status.text = "Could not open the saved conversation. It has not been changed." } }
        }
    }
    private fun message(who: String, body: String) {
        transcript.addView(TextView(this).apply {
            text = "$who\n$body"; textSize = 16f; setTextColor(Color.rgb(35,35,35))
            setTextIsSelectable(true); setPadding(0,dp(10),0,dp(10))
        })
        scroll.post { scroll.fullScroll(View.FOCUS_DOWN) }
    }
    private fun configure() {
        if (busy) return
        val form = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL; setPadding(dp(20),0,dp(20),0) }
        fun field(label: String, value: String, secret: Boolean = false): EditText {
            form.addView(TextView(this).apply { text = label })
            return EditText(this).apply {
                setText(value); isSingleLine = true; isSaveEnabled = false
                importantForAutofill = View.IMPORTANT_FOR_AUTOFILL_NO
                inputType = if(secret) InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_PASSWORD else InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_URI
                form.addView(this)
            }
        }
        val endpoint=field("OpenAI-compatible HTTPS base URL",prefs.getString("endpoint","").orEmpty())
        val model=field("Model name",prefs.getString("model","").orEmpty())
        val token=field("API key (until this screen closes)",key,true)
        form.addView(TextView(this).apply {
            text="Messages and requested app lists go to this provider. Provider charges apply. Your Micro account and credits are separate."; textSize=13f
        })
        AlertDialog.Builder(this).setTitle("Model connection").setView(form)
            .setPositiveButton("Use model") { _, _ ->
                prefs.edit().putString("endpoint",endpoint.text.toString().trim()).putString("model",model.text.toString().trim()).apply()
                key=token.text.toString(); status.text="Conversation saved on this phone. Model connected on request."
            }.setNegativeButton("Cancel",null).show()
    }
    private fun submit() {
        val question=input.text.toString().trim()
        if (busy || runtime == null || question.isEmpty()) return
        val endpoint=prefs.getString("endpoint","").orEmpty()
        val model=prefs.getString("model","").orEmpty()
        if(endpoint.isBlank() || model.isBlank()) { configure(); return }
        busy=true; send.isEnabled=false; settings.isEnabled=false; input.isEnabled=false
        status.text="Micro is working…"
        val credential=key
        worker.execute {
            try {
                val answer=runtime!!.ask(endpoint,credential,model,question)
                runOnUiThread { if(!isFinishing && !isDestroyed) {
                    message("You",question); message("Micro",answer); input.setText("")
                    status.text="Saved on this phone."; ready()
                } }
            } catch(e: Exception) {
                runOnUiThread { if(!isFinishing && !isDestroyed) {
                    status.text=e.message ?: "Request failed. Your message is still here."; ready()
                } }
            }
        }
    }
    private fun ready() { busy=false; send.isEnabled=true; settings.isEnabled=true; input.isEnabled=true }
    override fun onDestroy() { key=""; worker.shutdown(); super.onDestroy() }
    companion object { private var transportReady=false; private var sharedRuntime: mobile.Runtime? = null }
}
