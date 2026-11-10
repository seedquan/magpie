// Data folder (CHENGJIAHEGOXIS on X: magpie's files on an external SSD;
// buhaodeshiyehao on X): where magpie keeps its files and cache, moved into
// a folder on another disk and back (appdir/move.go). magpie restarts to
// move them, so nothing is moved under the running one.
let dataFolderView = null;

function dataFolderRows(d) {
  if (!d?.config) return [];
  const r = el("div", "row pref auto-backup data-folder");
  const who = el("div", "who");
  const sub = el("div", "sub wraps data-folder-dir", d.moved || d.config);
  who.append(el("div", "name", t("Data folder")), sub);
  const val = el("div", "val");
  const btn = (label, fn) => { const b = el("button", "text", label); b.type = "button"; b.onclick = (e) => fn(b, e); return b; };
  val.append(btn(t("Show"), () => api("datafolder/reveal", {}).catch(statusErr)));
  if (d.pending) val.append(btn(t("Cancel"), (b) => dataFolderDo("cancel", b)));
  else if (!d.portable) {
    val.append(btn(t("Move…"), () => dataFolderAsk(d, false)));
    if (d.moved) val.append(btn(t("Move back"), () => dataFolderAsk(d, true)));
  }
  r.append(who, val);
  const said = [];
  if (d.pending) said.push([d.pending === "home" ? t("Moves back to this computer when magpie starts again") : t("Moves to {dir} when magpie starts again", { dir: d.pending })]);
  if (d.pending && d.other) said.push([t("Another magpie serves the gateway (magpie serve): stop it, then open magpie again to move them")]);
  if (d.error) said.push([t("The last move wasn't made: {error}", { error: d.error }), "bad"]);
  sub.after(...said.map(([s, bad]) => el("div", "sub wraps data-folder-said" + (bad ? " bad" : ""), s)));
  return [r];
}

async function dataFolderDo(action, b) {
  b?.classList.add("busy");
  try {
    dataFolderView = (await api("datafolder/" + action, {})).view;
  } catch (e) {
    statusErr(e);
  }
  dataFolderRefresh();
}

// dataFolderAsk says what a move does before the folder is picked (or,
// back, before it is made): magpie restarts for it.
function dataFolderAsk(d, back) {
  const ed = el("div", "editor restore-ask data-folder-ask");
  const head = el("div", "ehead");
  head.append(el("b", "", back ? t("Move magpie's files back to this computer?") : t("Move magpie's files to another disk?")));
  ed.append(head);
  if (back) ed.append(el("p", "lib-confirm", t("They go back to {dir}, and the folder on the other disk is removed. magpie restarts to move them.", { dir: d.config })));
  else {
    ed.append(el("p", "lib-confirm", t("Its files and cache go into a folder named magpie in the folder you pick, and a link is left where they were, so agents still find them. magpie restarts to move them.")));
    ed.append(el("p", "lib-confirm", t("Keep that disk connected: without it magpie won't open until it is back.")));
  }
  const err = el("p", "editor-error", "");
  ed.append(err);
  const bar = el("div", "bar");
  const go = el("button", "text primary", back ? t("Move back") : t("Choose a folder…"));
  go.onclick = async (e) => {
    e.stopPropagation();
    go.disabled = true;
    go.classList.add("busy");
    try {
      const r = await api(back ? "datafolder/home" : "datafolder/move", {});
      dataFolderView = r.view;
      if (r.restart) { go.textContent = t("Restarting…"); return; }
      closeConfirmAsk();
      dataFolderRefresh();
    } catch (x) {
      go.disabled = false;
      go.classList.remove("busy");
      err.textContent = x.message;
    }
  };
  const cancel = el("button", "text", t("Cancel"));
  cancel.onclick = cancelAsk;
  bar.append(el("span", "grow"), cancel, go);
  ed.append(bar);
  confirmAsk = ed;
  openModal(ed);
  $("#modal").classList.add("lib");
  cancel.focus(NO_SCROLL);
}


window.dataFolderFill = async function (at) {
 const draw = () => { if (at.isConnected) at.replaceChildren(...dataFolderRows(dataFolderView)); };
 draw();
 try { dataFolderView = await api("datafolder"); draw(); } catch {}
};
function dataFolderRefresh() { const at = document.querySelector("#dataFolderList"); if (at) dataFolderFill(at); }
