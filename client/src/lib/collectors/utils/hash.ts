/**
 * 哈希工具函数
 * 用于将指纹数据压缩成唯一的短哈希值
 */

/**
 * cyrb53 哈希算法
 * 高性能、低碰撞率的非加密哈希函数
 * @param str 输入字符串
 * @param seed 可选种子值
 * @returns 53位数字哈希值
 */
export const cyrb53 = (str: string, seed = 0): number => {
  let h1 = 0xdeadbeef ^ seed;
  let h2 = 0x41c6ce57 ^ seed;

  for (let i = 0; i < str.length; i++) {
    const ch = str.charCodeAt(i);
    h1 = Math.imul(h1 ^ ch, 2654435761);
    h2 = Math.imul(h2 ^ ch, 1597334677);
  }

  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909);
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909);

  return 4294967296 * (2097151 & h2) + (h1 >>> 0);
};

/**
 * 生成十六进制哈希字符串
 * @param data 输入数据
 * @param seed 可选种子值
 * @returns 十六进制哈希字符串
 */
export const toHexHash = (data: string, seed = 0): string => {
  return cyrb53(data, seed).toString(16);
};
