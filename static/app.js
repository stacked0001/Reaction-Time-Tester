'use strict';

// --- State ---
let username = null;
let ws = null;
let gameState = 'waiting';
let canClick = false;

// --- DOM refs ---
const authDiv    = document.getElementById('auth');
const gameDiv    = document.getElementById('game');
const usernameIn = document.getElementById('username');
const passwordIn = document.getElementById('password');
const errorMsg   = document.getElementById('error-msg');
const playerCount= document.getElementById('player-count');
const statusEl   = document.getElementById('status');
const clickZone  = document.getElementById('click-zone');
const resultsBox = document.getElementById('results-box');
const winnerLabel= document.getElementById('winner-label');

// --- Auth ---

document.getElementById('btn-login').addEventListener('click', () => doAuth('/login'));
document.getElementById('btn-register').addEventListener('click', () => doAuth('/register'));

async function doAuth(endpoint) {
  const user = usernameIn.value.trim();
  const pass = passwordIn.value.trim();
  if (!user || !pass) { showError('Enter username and password.'); return; }

  try {
    const res = await fetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: user, password: pass })
    });

    const text = await res.text();

    if (!res.ok) {
      showError(text.trim() || 'Error.');
      return;
    }

    username = user;
    showGame();
    connectWS();
  } catch (e) {
    showError('Network error.');
  }
}

function showError(msg) {
  errorMsg.textContent = msg;
}

// --- Game UI ---

function showGame() {
  authDiv.style.display = 'none';
  gameDiv.style.display = 'block';
}

function setStatus(text) {
  statusEl.textContent = text;
}

function setClickable(on) {
  canClick = on;
  if (on) {
    clickZone.classList.add('ready');
    clickZone.classList.remove('disq');
    clickZone.textContent = 'CLICK!';
  } else {
    clickZone.classList.remove('ready');
    clickZone.textContent = 'wait...';
  }
}

clickZone.addEventListener('click', () => {
  if (!ws || ws.readyState !== WebSocket.OPEN) return;
  if (gameState === 'pending' || gameState === 'active') {
    ws.send(JSON.stringify({ type: 'click' }));
    setClickable(false);
    clickZone.style.cursor = 'not-allowed';
  }
});

// --- WebSocket ---

function connectWS() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  ws = new WebSocket(`${proto}://${location.host}/ws?username=${encodeURIComponent(username)}`);

  ws.onopen = () => {
    setStatus('Waiting for players...');
    setClickable(false);
    resultsBox.innerHTML = '';
    winnerLabel.textContent = '';
  };

  ws.onmessage = (e) => {
    let msg;
    try { msg = JSON.parse(e.data); } catch { return; }
    handleMessage(msg);
  };

  ws.onclose = () => {
    setStatus('Disconnected. Refresh to reconnect.');
    setClickable(false);
  };

  ws.onerror = () => {
    setStatus('Connection error.');
  };
}

function handleMessage(msg) {
  switch (msg.type) {

    case 'players':
      playerCount.textContent = `${msg.payload} player${msg.payload !== 1 ? 's' : ''} connected`;
      break;

    case 'state':
      gameState = msg.payload;
      resultsBox.innerHTML = '';
      winnerLabel.textContent = '';
      clickZone.classList.remove('disq', 'ready');
      clickZone.style.cursor = 'not-allowed';

      if (msg.payload === 'waiting') {
        setStatus('Waiting for players...');
        setClickable(false);
        clickZone.textContent = 'wait...';
      } else if (msg.payload === 'pending') {
        setStatus('Get ready...');
        setClickable(false);
        clickZone.textContent = 'wait...';
      } else if (msg.payload === 'results') {
        setClickable(false);
      }
      break;

    case 'go':
      gameState = 'active';
      setStatus('GO!');
      setClickable(true);
      clickZone.style.cursor = 'pointer';
      break;

    case 'your_result':
      setStatus(`Your reaction: ${msg.payload}ms`);
      setClickable(false);
      clickZone.textContent = `${msg.payload}ms`;
      clickZone.style.cursor = 'not-allowed';
      break;

    case 'disqualified':
      gameState = 'disqualified';
      setStatus('Disqualified!');
      setClickable(false);
      clickZone.classList.remove('ready');
      clickZone.classList.add('disq');
      clickZone.textContent = msg.payload || 'disqualified';
      clickZone.style.cursor = 'not-allowed';
      break;

    case 'results':
      gameState = 'results';
      showResults(msg.payload);
      break;
  }
}

function showResults(data) {
  const { results, winner } = data;
  if (!results) return;

  // Sort: valid times first (ascending), then disqualified
  results.sort((a, b) => {
    if (a.disqualified && !b.disqualified) return 1;
    if (!a.disqualified && b.disqualified) return -1;
    return a.reaction_ms - b.reaction_ms;
  });

  resultsBox.innerHTML = results.map(r => {
    const isWinner = r.username === winner;
    const cls = r.disqualified ? 'result-row disq-row' : isWinner ? 'result-row winner' : 'result-row';
    const time = r.disqualified ? 'DQ' : `${r.reaction_ms}ms`;
    return `<div class="${cls}"><span>${r.username}</span><span>${time}</span></div>`;
  }).join('');

  if (winner) {
    winnerLabel.textContent = `Winner: ${winner}`;
    setStatus('Round over!');
  } else {
    winnerLabel.textContent = 'No winner this round.';
    setStatus('Round over!');
  }
}
