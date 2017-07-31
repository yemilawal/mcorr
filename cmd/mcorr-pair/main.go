package main

import (
	"bufio"
	"io"
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/alecthomas/kingpin"
	"github.com/mingzhi/biogo/seq"
	"github.com/mingzhi/mcorr"
	"github.com/mingzhi/ncbiftp/taxonomy"
)

func main() {
	app := kingpin.New("mcorr-pair", "Calculate mutation correlation for each pair of isolates")
	app.Version("v20170728")

	alnFile := app.Arg("in", "Alignment file in XMFA format").Required().String()
	outFile := app.Arg("out", "Output file in CSV format").Required().String()

	mateFile := app.Flag("second-alignment", "Second alignment file in XMFA format").Default("").String()
	maxl := app.Flag("max-corr-length", "Maximum length of correlation (base pairs)").Default("300").Int()
	ncpu := app.Flag("num-cpu", "Number of CPUs (default: using all available cores)").Default("0").Int()
	numBoot := app.Flag("num-boot", "Number of bootstrapping on genes").Default("1000").Int()
	codonPos := app.Flag("codon-position", "Codon position (1: first codon position; 2: second codon position; 3: third codon position; 4: synoumous at third codon position.").Default("4").Int()
	kingpin.MustParse(app.Parse(os.Args[1:]))

	if *ncpu == 0 {
		*ncpu = runtime.NumCPU()
	}
	runtime.GOMAXPROCS(*ncpu)

	synoumous := false
	if *codonPos == 4 {
		synoumous = true
		*codonPos = 3
	}
	if *codonPos <= 0 || *codonPos > 4 {
		log.Fatalln("--codon-position should be in the range of 1 to 4.")
	}

	var mateMap map[string]*seq.Sequence
	if *mateFile != "" {
		f, err := os.Open(*mateFile)
		if err != nil {
			panic(err)
		}
		rd := seq.NewFastaReader(f)
		sequences, err := rd.ReadAll()
		if err != nil {
			panic(err)
		}

		mateMap = make(map[string]*seq.Sequence)
		for _, s := range sequences {
			geneid := strings.Split(s.Id, " ")[0]
			mateMap[geneid] = s
		}

		f.Close()
	}

	alnChan := readAlignments(*alnFile)

	codingTable := taxonomy.GeneticCodes()["11"]
	maxCodonLen := *maxl / 3
	codonOffset := 0

	numJob := *ncpu
	done := make(chan bool)
	resChan := make(chan mcorr.CorrResults)
	for i := 0; i < numJob; i++ {
		go func() {
			for aln := range alnChan {
				var mateSequence *seq.Sequence
				if mateMap != nil {
					geneid, _ := getNames(aln.Sequences[0].Id)
					s, found := mateMap[geneid]
					if found {
						mateSequence = s
					}
				}
				corrRes := calcP2Coding(aln, codonOffset, maxCodonLen, codingTable, synoumous, *codonPos-1, mateSequence)
				for _, res := range corrRes {
					resChan <- res
				}
			}
			done <- true
		}()
	}

	go func() {
		defer close(resChan)
		for i := 0; i < numJob; i++ {
			<-done
		}
	}()

	mcorr.CollectWrite(resChan, *outFile, *numBoot)
}

// Alignment is an array of multiple sequences with same length
type Alignment struct {
	ID        string
	Sequences []seq.Sequence
}

// readAlignments reads sequence alignment from a extended Multi-FASTA file,
// and return a channel of alignment, which is a list of seq.Sequence
func readAlignments(file string) (alnChan chan Alignment) {
	alnChan = make(chan Alignment)
	read := func() {
		defer close(alnChan)

		f, err := os.Open(file)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		xmfaReader := seq.NewXMFAReader(f)
		for {
			alignment, err := xmfaReader.Read()
			if err != nil {
				if err != io.EOF {
					panic(err)
				}
				break
			}
			if len(alignment) > 0 {
				alnID := strings.Split(alignment[0].Id, " ")[0]
				alnChan <- Alignment{ID: alnID, Sequences: alignment}
			}
		}
	}
	go read()
	return
}

func calcP2Coding(aln Alignment, codonOffset int, maxCodonLen int, codingTable *taxonomy.GeneticCode, synonymous bool, codonPos int, mateSequence *seq.Sequence) (results []mcorr.CorrResults) {
	codonSequences := [][]Codon{}
	sequences := []seq.Sequence{}
	if mateSequence != nil {
		sequences = append(sequences, *mateSequence)
	}
	sequences = append(sequences, aln.Sequences...)
	for _, s := range sequences {
		codons := extractCodons(s, codonOffset)
		codonSequences = append(codonSequences, codons)
	}

	for i, seq1 := range codonSequences {
		for j := i + 1; j < len(codonSequences); j++ {
			_, genomeName1 := getNames(aln.Sequences[i].Id)
			_, genomeName2 := getNames(aln.Sequences[j].Id)
			if genomeName1 > genomeName2 {
				genomeName1, genomeName2 = genomeName2, genomeName1
			}
			id := genomeName1 + "_vs_" + genomeName2
			seq2 := codonSequences[j]
			crRes := mcorr.CorrResults{ID: id}
			for l := 0; l < maxCodonLen; l++ {
				d := 0.0
				t := 0
				for k := 0; k < len(seq1)-l; k++ {
					c1 := seq1[k]
					c2 := seq2[k]
					a1, found1 := codingTable.Table[string(c1)]
					a2, found2 := codingTable.Table[string(c2)]
					if found1 && found2 && a1 == a2 {
						b1 := seq1[k+l]
						b2 := seq2[k+l]

						good := true
						if synonymous {
							d1, found1 := codingTable.Table[string(c1)]
							d2, found2 := codingTable.Table[string(c2)]
							if found1 && found2 && d1 == d2 {
								good = true
							} else {
								good = false
							}
						}
						if good {
							var codonPositions []int
							if codonPos < 0 || codonPos > 2 {
								codonPositions = []int{0, 1, 2}
							} else {
								codonPositions = append(codonPositions, codonPos)
							}
							for _, codonP := range codonPositions {
								if c1[codonP] != c2[codonP] {
									if b1[codonP] != b2[codonP] {
										d++
									}
								}
								t++
							}
						}
					}
				}
				cr := mcorr.CorrResult{}
				cr.Lag = l * 3
				cr.Mean = d / float64(t)
				cr.N = t
				cr.Type = "P2"
				crRes.Results = append(crRes.Results, cr)
			}
			results = append(results, crRes)
		}
		if mateSequence != nil {
			break
		}
	}

	return
}

// Codon is a byte list of length 3
type Codon []byte

// CodonSequence is a sequence of codons.
type CodonSequence []Codon

// CodonPair is a pair of Codons.
type CodonPair struct {
	A, B Codon
}

// extractCodons return a list of codons from a DNA sequence.
func extractCodons(s seq.Sequence, offset int) (codons []Codon) {
	for i := offset; i+3 <= len(s.Seq); i += 3 {
		c := s.Seq[i:(i + 3)]
		codons = append(codons, c)
	}
	return
}

// countAlignments return total number of alignments in a file.
func countAlignments(file string) (count int) {
	f, err := os.Open(file)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				panic(err)
			}
			break
		}
		if line[0] == '=' {
			count++
		}
	}
	return
}

func getNames(s string) (geneName, genomeName string) {
	terms := strings.Split(s, " ")
	geneName = terms[0]
	genomeName = terms[1]
	return
}
