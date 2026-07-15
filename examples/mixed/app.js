// Application script for the mixed Go+HTML+CSS+JS demo.
//
// This script is run through wb-ui's JavaScript runtime (wb-ui/jsc) with the DOM
// bindings (wb-ui/bindings) installed, so `document` resolves to the loaded
// page and `go.GetTime` / `go.Calculate` resolve to Go functions registered by
// the host via bindings.RegisterGoFunction. The script demonstrates the JS ->
// Go direction of the bridge: it calls into Go and renders the result back into
// the DOM.

// updateDisplay writes text into the element with the given id. The `document`
// object is installed by bindings.RegisterDOMBindings and routes through to the
// real dom.Document on the Go side.
function updateDisplay(id, text) {
  var el = document.getElementById(id);
  if (el) {
    el.textContent = text;
    return true;
  }
  return false;
}

// handleClick is what a real browser would fire on the button's click event.
// wb-ui has no event loop, so the host calls it directly after registration; the
// structure still mirrors an event-driven page so the demo stays faithful to a
// real web app.
function handleClick() {
  // Call into Go: go.GetTime() returns the current host time as a string.
  var now = go.GetTime();
  updateDisplay("time-display", "Server time: " + now);

  // Call into Go: go.Calculate(a, b) multiplies two numbers on the host and
  // returns the result. This shows arbitrary argument passing across the bridge.
  var product = go.Calculate(6, 7);
  updateDisplay("calc-result", "go.Calculate(6, 7) = " + product);

  console.log("JS: handled click, time=" + now + " product=" + product);
  return product;
}

// Invoke the handler once on load so the demo produces visible output without an
// actual click. In a real browser this would be inside an addEventListener call.
handleClick();
