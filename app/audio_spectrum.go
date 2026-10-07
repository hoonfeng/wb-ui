package app

// 音频频谱工具（主线 A3-1 的判据 A 用）。
//
// 判据 A 是「宿主输出回调的 PCM 做 FFT，主峰 ≈ 样本频率（440Hz）」：这是
// 「引擎确实把音频交给了输出」的自证判据——不需要环回录音设备（判据 B 的
// 硬件前提多数机器不满足）。因此这段 FFT 是**验证工具**的一部分，不是引擎或
// 播放链路的一部分：播放路径上没有任何频谱计算。
//
// 实现选择：迭代式 radix-2 FFT（几十行、无第三方依赖）。这里只需要「找出主峰
// 频率」，不需要高精度/实时性——所以不做重叠、不做多窗、不做峰值插值之外的补偿。
import (
	"math"
)

// spectrumMaxSamples 是参与 FFT 的最大样本数（2 的幂）。16384 点 @48kHz
// 约 341ms、频率分辨率 ≈ 2.93Hz——足以把 440Hz 与相邻谐波/噪声分开，又小到
// 一次 1 秒样本里能取到完整窗口。
const spectrumMaxSamples = 16384

// PCMToMono 把 s16le 交错 PCM 转成单声道 float64（按声道取平均）。
func PCMToMono(data []byte, channels int) []float64 {
	if channels < 1 {
		channels = 1
	}
	frames := len(data) / (2 * channels)
	out := make([]float64, 0, frames)
	for f := 0; f < frames; f++ {
		sum := 0.0
		for c := 0; c < channels; c++ {
			i := (f*channels + c) * 2
			v := int16(uint16(data[i]) | uint16(data[i+1])<<8)
			sum += float64(v)
		}
		out = append(out, sum/float64(channels))
	}
	return out
}

// DominantFrequency 返回 samples 的频谱主峰频率（Hz）与归一化幅度（0~1）。
// 样本不足两个点、或全是直流时返回 (0, 0)。
//
// 步骤：取 2 的幂长度窗口 → 去直流 → Hann 窗（减少泄漏，避免 440Hz 的旁瓣
// 盖过真实峰值）→ radix-2 FFT → 找幅度最大 bin → 抛物插值细化。
func DominantFrequency(samples []float64, sampleRate int) (float64, float64) {
	if sampleRate <= 0 || len(samples) < 4 {
		return 0, 0
	}
	n := 1
	for n*2 <= len(samples) && n*2 <= spectrumMaxSamples {
		n *= 2
	}
	if n < 4 {
		return 0, 0
	}
	re := make([]float64, n)
	im := make([]float64, n)
	mean := 0.0
	for i := 0; i < n; i++ {
		mean += samples[i]
	}
	mean /= float64(n)
	for i := 0; i < n; i++ {
		// Hann 窗：w = 0.5(1 - cos(2πi/(N-1)))
		w := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
		re[i] = (samples[i] - mean) * w
	}
	fftInPlace(re, im)

	half := n / 2
	peak, peakMag := 0, 0.0
	mags := make([]float64, half+1)
	for k := 0; k <= half; k++ {
		m := math.Hypot(re[k], im[k])
		mags[k] = m
		if m > peakMag {
			peakMag, peak = m, k
		}
	}
	if peakMag <= 0 {
		return 0, 0
	}
	// 抛物插值：真实峰常落在两个 bin 之间（440Hz 在 16384 点 @48k 下不是整数 bin）。
	bins := float64(peak)
	if peak > 0 && peak < half {
		l, c, r := mags[peak-1], mags[peak], mags[peak+1]
		denom := l - 2*c + r
		if denom != 0 {
			delta := 0.5 * (l - r) / denom
			if delta > -1 && delta < 1 {
				bins += delta
			}
		}
	}
	freq := bins * float64(sampleRate) / float64(n)
	norm := peakMag / (float64(n) * 0.25) // Hann 窗的相干增益 ≈ 0.5，幅度折算到 0~1 量级
	if norm > 1 {
		norm = 1
	}
	return freq, norm
}

// fftInPlace 是就地迭代 radix-2 FFT（len 必须是 2 的幂；原地覆盖 re/im）。
func fftInPlace(re, im []float64) {
	n := len(re)
	if n <= 1 {
		return
	}
	// 位反转置换。
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j &^= bit
		}
		j |= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := -2 * math.Pi / float64(length)
		wr, wi := math.Cos(ang), math.Sin(ang)
		half := length / 2
		for i := 0; i < n; i += length {
			cr, ci := 1.0, 0.0
			for k := 0; k < half; k++ {
				a := i + k
				b := a + half
				vr := re[b]*cr - im[b]*ci
				vi := re[b]*ci + im[b]*cr
				re[b], im[b] = re[a]-vr, im[a]-vi
				re[a], im[a] = re[a]+vr, im[a]+vi
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
}
