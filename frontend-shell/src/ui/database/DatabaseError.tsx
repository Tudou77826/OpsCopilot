import { useEffect, useState } from 'react';
import { TbCopy, TbCheck } from 'react-icons/tb';

export function DatabaseError({ message }: { message: string }) {
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  useEffect(() => {
    setCopied(false);
    setCopyFailed(false);
  }, [message]);
  async function copy() {
    try {
      await navigator.clipboard.writeText(message);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopyFailed(true);
    }
  }
  return (
    <div className="oc-db-error oc-db-error-detail" role="alert">
      <span>
        {message}
        {copyFailed && <small>复制失败，请选中错误文本复制。</small>}
      </span>
      <button
        type="button"
        aria-label={copied ? '错误详情已复制' : '复制错误详情'}
        title={copied ? '已复制' : '复制错误详情'}
        onClick={() => void copy()}
      >
        {copied ? <TbCheck size={15} /> : <TbCopy size={15} />}
      </button>
    </div>
  );
}
