/** Match only complete protocol replies, never mixed input or cursor keys. */
export function isTerminalStatusReply(data: string): boolean {
    const match = /^\x1b\[(?:\??[0-9]+;[0-9]+R|[0-9]+n|[?>][0-9;]+c)$/.exec(data);
    // JavaScript's $ also matches before a final newline; that newline would
    // be user input, so require an exact whole-string match as the Go gate does.
    return match !== null && match[0] === data;
}
