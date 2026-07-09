export function isModifiedPointerEvent(event) {
    return event.button !== 0
        || event.metaKey
        || event.ctrlKey
        || event.shiftKey
        || event.altKey;
}
